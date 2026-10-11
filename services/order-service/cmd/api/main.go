// Command api arranca el Order Service: HTTP para health checks (BE-T4a).
// BE-T4b suma gRPC y el dominio Customer.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// BE-T4b: wiring del dominio Customer. Por ahora los casos de uso se
	// construyen pero no se exponen en endpoints (eso entra en BE-T5 con
	// los handlers gRPC). Mantenerlos vivos acá evita el warning de
	// variables no usadas y deja el grafo de dependencias listo.
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

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           httpx.Handler(pool.Ping),
	}

	errCh := make(chan error, 1)
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
	return nil
}
