// Command api arranca el Routing Service: conecta los clientes gRPC de Order, Zone y Driver
// Service, construye el caso de uso de generación de rutas y sirve routing.v1 por gRPC.
// No tiene base de datos: el estado vive en los servicios que consume.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/adapters/clients"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/plan"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/infrastructure/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "routing-service")
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

	orderConn, err := dial(cfg.OrderServiceAddr)
	if err != nil {
		return fmt.Errorf("dial order service: %w", err)
	}
	defer func() { _ = orderConn.Close() }()

	zoneConn, err := dial(cfg.ZoneServiceAddr)
	if err != nil {
		return fmt.Errorf("dial zone service: %w", err)
	}
	defer func() { _ = zoneConn.Close() }()

	driverConn, err := dial(cfg.DriverServiceAddr)
	if err != nil {
		return fmt.Errorf("dial driver service: %w", err)
	}
	defer func() { _ = driverConn.Close() }()

	generate := plan.New(
		clients.NewOrderGateway(ordersv1.NewOrderServiceClient(orderConn)),
		clients.NewZoneCapacities(zonesv1.NewZoneServiceClient(zoneConn)),
		clients.NewDriverPool(driversv1.NewDriverServiceClient(driverConn)),
		domain.ZoneCapacityPlanner{},
	)
	log.Info("routing service wired", "orderService", cfg.OrderServiceAddr,
		"zoneService", cfg.ZoneServiceAddr, "driverService", cfg.DriverServiceAddr)

	return serveGRPC(ctx, log, cfg.GRPCPort, grpcadapter.NewRoutingServer(generate))
}

// dial creates a lazy client connection; the connection is established on the first call.
func dial(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// serveGRPC exposes routing.v1 until ctx is cancelled, then stops gracefully.
func serveGRPC(ctx context.Context, log *slog.Logger, port int, server routingv1.RoutingServiceServer) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}
	srv := grpc.NewServer()
	routingv1.RegisterRoutingServiceServer(srv, server)
	log.Info("grpc listening", "port", port)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(lis) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		srv.GracefulStop()
		return nil
	}
}
