// Command api arranca el Driver Service: aplica sus migraciones, abre el pool de PostgreSQL
// y sirve los endpoints de salud. Las rutas de repartidores se conectan a los casos de uso
// en un paso posterior; por ahora este proceso solo prueba la base y el arranque.
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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/config"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/postgres"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "driver-service")
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

	// Los casos de uso aún no se exponen por HTTP; se cablean cuando existan las rutas de
	// repartidores. Aquí solo se registra el radio con el que arranca el proceso.
	log.Info("driver service ready to wire use cases", "arrivalRadiusMeters", cfg.ArrivalRadiusMeters)

	return serve(ctx, log, cfg.HTTPPort, pool)
}

func serve(ctx context.Context, log *slog.Logger, port int, pool *pgxpool.Pool) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Info("http listening", "port", port)

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		// El contexto ya está cancelado; WithoutCancel conserva los valores y da tiempo al apagado.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
