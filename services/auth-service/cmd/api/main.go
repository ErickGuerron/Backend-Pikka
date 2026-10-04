// Command api arranca el Auth Service: gRPC para tráfico interno y HTTP para health checks.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/grpc"
	httpadapter "github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/config"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/logging"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/postgres"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/security"
)

const serviceName = "auth-service"

func main() {
	log := logging.New(serviceName, os.Getenv("LOG_LEVEL"))
	if err := run(log); err != nil {
		log.Error("service stopped with error", "error", err.Error())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := cfg.Postgres.DSN()
	if err := postgres.Migrate(dsn); err != nil {
		return err
	}
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	users := persistence.NewUserRepository(pool)
	hasher := security.NewBcryptHasher(cfg.BcryptCost)
	tokens := security.NewJWTIssuer(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)

	login, err := application.NewLogin(users, hasher, tokens)
	if err != nil {
		return err
	}

	if cfg.BootstrapAdminEmail != "" {
		created, err := application.NewBootstrapAdmin(users, hasher).Execute(ctx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword)
		if err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
		if created {
			log.Info("bootstrap admin created")
		}
	}

	grpcServer := grpc.NewServer(
		grpcadapter.UnaryInterceptors(log),
		grpc.KeepaliveParams(keepalive.ServerParameters{MaxConnectionIdle: 5 * time.Minute}),
	)
	authv1.RegisterAuthServiceServer(grpcServer, grpcadapter.NewServer(grpcadapter.UseCases{
		Login:         login,
		CreateUser:    application.NewCreateUser(users, hasher),
		GetUser:       application.NewGetUser(users),
		ValidateToken: application.NewValidateToken(tokens),
	}))
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus(authv1.AuthService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	httpServer := httpadapter.NewHealthServer(pool.Ping)
	httpServer.Server.ReadHeaderTimeout = 5 * time.Second

	errCh := make(chan error, 2)
	go func() {
		lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
		if err != nil {
			errCh <- err
			return
		}
		log.Info("grpc listening", "port", cfg.GRPCPort)
		errCh <- grpcServer.Serve(lis)
	}()
	go func() {
		log.Info("http listening", "port", cfg.HTTPPort)
		if err := httpServer.Start(fmt.Sprintf(":%d", cfg.HTTPPort)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		return err
	}

	healthSrv.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)

	done := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
	return nil
}
