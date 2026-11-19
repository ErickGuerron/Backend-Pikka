// Command api arranca el Zone Service: gRPC para tráfico interno y HTTP para health checks.
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

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/infrastructure/config"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/infrastructure/postgres"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "zone-service")
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

	zones := application.NewZoneService(persistence.NewZoneRepository(pool))

	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{MaxConnectionIdle: 5 * time.Minute}),
	)
	zonesv1.RegisterZoneServiceServer(grpcServer, grpcadapter.NewServer(zones))
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus(zonesv1.ZoneService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           healthHandler(pool.Ping),
	}

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
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// healthHandler expone /health (proceso vivo) y /ready (base de datos alcanzable).
func healthHandler(ping func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"status":%q}`, body["status"])
}
