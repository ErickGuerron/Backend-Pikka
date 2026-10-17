// Command api arranca el API Gateway: único punto de entrada REST/JSON del sistema.
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

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/config"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/logging"
)

const serviceName = "api-gateway"

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

	authConn, err := grpcclient.Dial(cfg.AuthServiceAddr)
	if err != nil {
		return fmt.Errorf("dial auth service: %w", err)
	}
	defer func() { _ = authConn.Close() }()
	auth := grpcclient.NewAuth(authConn, cfg.GRPCTimeout)

	zoneConn, err := grpcclient.Dial(cfg.ZoneServiceAddr)
	if err != nil {
		return fmt.Errorf("dial zone service: %w", err)
	}
	defer func() { _ = zoneConn.Close() }()
	zones := grpcclient.NewZones(zoneConn, cfg.GRPCTimeout)

	orderConn, err := grpcclient.Dial(cfg.OrderServiceAddr)
	if err != nil {
		return fmt.Errorf("dial order service: %w", err)
	}
	defer func() { _ = orderConn.Close() }()
	customers := grpcclient.NewCustomer(orderConn, cfg.GRPCTimeout)
	orders := grpcclient.NewOrders(orderConn, cfg.GRPCTimeout)
	catalog := grpcclient.NewCatalog(orderConn, cfg.GRPCTimeout)

	driverConn, err := grpcclient.Dial(cfg.DriverServiceAddr)
	if err != nil {
		return fmt.Errorf("dial driver service: %w", err)
	}
	defer func() { _ = driverConn.Close() }()
	drivers := grpcclient.NewDrivers(driverConn, cfg.GRPCTimeout)

	trackingConn, err := grpcclient.Dial(cfg.TrackingServiceAddr)
	if err != nil {
		return fmt.Errorf("dial tracking service: %w", err)
	}
	defer func() { _ = trackingConn.Close() }()
	tracking := grpcclient.NewTracking(trackingConn, cfg.GRPCTimeout)

	routingConn, err := grpcclient.Dial(cfg.RoutingServiceAddr)
	if err != nil {
		return fmt.Errorf("dial routing service: %w", err)
	}
	defer func() { _ = routingConn.Close() }()
	routing := grpcclient.NewRouting(routingConn, cfg.GRPCTimeout)

	e := httpapi.NewRouter(log, httpapi.Options{
		CORSAllowedOrigins:   cfg.CORSAllowedOrigins,
		RateLimitPerSecond:   cfg.RateLimitPerSecond,
		RateLimitBurst:       cfg.RateLimitBurst,
		LoginRatePerMinute:   cfg.LoginRatePerMinute,
		SignupRatePerMinute:  cfg.SignupRatePerMinute,
		RequestTimeout:       cfg.RequestTimeout,
		BodyLimit:            cfg.BodyLimit,
		TrustProxy:           cfg.TrustProxy,
		EnableHSTS:           cfg.EnableHSTS,
		EnableDocs:           cfg.EnableDocs,
		Customers:            customers,
		CustomerQueries:      customers,
		Orders:               orders,
		Catalog:              catalog,
		Drivers:              drivers,
		Tracking:             tracking,
		TrackingPollInterval: cfg.TrackingPollInterval,
		Routing:              routing,
		DriverCreator:        drivers,
	}, jwt.NewVerifier(cfg.JWTSecret, cfg.JWTIssuer), auth, zones, func(ctx context.Context) bool {
		// El gateway solo está listo si puede llegar a sus dependencias gRPC.
		return auth.Ready(ctx) && zones.Ready(ctx)
	})

	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 15 * time.Second
	e.Server.WriteTimeout = cfg.RequestTimeout + 5*time.Second
	e.Server.IdleTimeout = 60 * time.Second

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "port", cfg.HTTPPort)
		if err := e.Start(fmt.Sprintf(":%d", cfg.HTTPPort)); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	return e.Shutdown(shutdownCtx)
}
