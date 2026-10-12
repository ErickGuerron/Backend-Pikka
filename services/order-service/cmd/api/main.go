// Command api arranca el Order Service: HTTP para health checks (BE-T4a),
// gRPC para exponer el dominio Customer (BE-T5a). BE-T5b sumará los
// handlers REST que el API Gateway consume contra este server gRPC.
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

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/grpc"
	httpx "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	customercase "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/customer"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/infrastructure/config"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/infrastructure/postgres"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "order-service")
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

	// BE-T4b: wiring del dominio Customer. Los casos de uso se construyen y
	// se exponen vía gRPC en este mismo proceso (BE-T5a).
	customerRepo := persistence.NewCustomerRepository(pool)
	createCustomer := customercase.NewCreateCustomer(customerRepo)
	getByUserID := customercase.NewGetByUserID(customerRepo)
	updateCustomerStatus := customercase.NewUpdateStatus(customerRepo)
	canCreateOrders := customercase.NewCanCreateOrders(customerRepo)
	log.Info("customer use cases wired",
		"create", createCustomer != nil,
		"getByUserID", getByUserID != nil,
		"updateStatus", updateCustomerStatus != nil,
		"canCreateOrders", canCreateOrders != nil,
	)

	// gRPC server para el dominio Customer. BE-T5b va a sumar interceptores
	// de autenticación/autorización (ADR 0008 §3) cuando el gateway exponga
	// los endpoints REST; por ahora el server solo traduce.
	grpcServer := grpc.NewServer()
	customerv1.RegisterCustomerServiceServer(grpcServer, grpcadapter.NewCustomerServer(grpcadapter.UseCases{
		Create:          createCustomer,
		GetByUserID:     getByUserID,
		UpdateStatus:    updateCustomerStatus,
		CanCreateOrders: canCreateOrders,
	}))

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           httpx.Handler(pool.Ping),
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
