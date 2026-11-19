// Command api arranca el Tracking Service: guarda la última posición de cada repartidor en Redis y
// estima distancia y tiempo hasta un destino con OSRM. Solo expone gRPC interno; no tiene puerto HTTP
// ni se publica al host.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/adapters/grpc"
	osrmadapter "github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/adapters/osrm"
	redisadapter "github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/adapters/redis"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/application/position"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/infrastructure/config"
)

// osrmTimeout limita cada consulta de ruta: un motor lento no debe bloquear la respuesta de ETA.
const osrmTimeout = 3 * time.Second

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "tracking-service")
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

	// Sin Redis no hay posiciones que servir: falla al arrancar en lugar de responder errores en cada llamada.
	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	defer func() { _ = rdb.Close() }()
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err = rdb.Ping(pingCtx).Err()
	cancelPing()
	if err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	store := redisadapter.NewPositionStore(rdb)
	estimator := osrmadapter.NewEstimator(cfg.OSRMURL, osrmTimeout)
	tracker := position.NewTracker(store, estimator, time.Now)

	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}
	srv := grpc.NewServer()
	grpcadapter.Register(srv, tracker)
	log.Info("grpc listening", "port", cfg.GRPCPort)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(lis) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		srv.GracefulStop()
		return <-errCh
	}
}
