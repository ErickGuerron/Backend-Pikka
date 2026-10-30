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
	_ "time/tzdata" // zona horaria operativa disponible sin depender del sistema operativo

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/geocoding"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/grpc"
	httpx "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/payments"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	zonesclient "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/zones"
	catalogcase "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	customercase "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/customer"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
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
	getCustomerByID := customercase.NewGetByID(customerRepo)
	listCustomers := customercase.NewListCustomers(customerRepo)
	getByUserID := customercase.NewGetByUserID(customerRepo)
	updateCustomerStatus := customercase.NewUpdateStatus(customerRepo)
	canCreateOrders := customercase.NewCanCreateOrders(customerRepo)
	log.Info("customer use cases wired",
		"create", createCustomer != nil,
		"getByID", getCustomerByID != nil,
		"list", listCustomers != nil,
		"getByUserID", getByUserID != nil,
		"updateStatus", updateCustomerStatus != nil,
		"canCreateOrders", canCreateOrders != nil,
	)

	// Pedidos (ADR 0007): validación automática con Zone Service y cliente registrado.
	location, err := time.LoadLocation(cfg.OperatingTimezone)
	if err != nil {
		return fmt.Errorf("operating timezone: %w", err)
	}
	zoneConn, err := grpc.NewClient(cfg.ZoneServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial zone service: %w", err)
	}
	defer func() { _ = zoneConn.Close() }()
	orderRepo := persistence.NewOrderRepository(pool)
	notificationRepo := persistence.NewNotificationRepository(pool)
	zoneClassifier := zonesclient.NewClassifier(zoneConn, cfg.ZoneTimeout)
	zoneQueue := orderapp.NewZoneQueue(orderRepo, zoneClassifier)
	geocoder := geocoding.NewPhoton(cfg.PhotonURL, cfg.PhotonBBox, cfg.GeocoderTimeout)
	catalogStore := persistence.NewStore(pool)
	createOrder := orderapp.NewCreateOrder(customerRepo, zoneClassifier, geocoder, catalogStore, catalogStore, location).WithZoneQueue(zoneQueue)
	listMyOrders := orderapp.NewListMyOrders(customerRepo, orderRepo).WithZoneQueue(zoneQueue)
	reviewOrder := orderapp.NewReviewOrder(customerRepo, zoneClassifier, geocoder, orderRepo, location)
	listExceptions := orderapp.NewListExceptions(orderRepo)
	listAllOrders := orderapp.NewListOrders(orderRepo)
	listMyNotifications := orderapp.NewListMyNotifications(customerRepo, notificationRepo)
	cancelOrder := orderapp.NewCancelOrder(customerRepo, orderRepo).WithZoneQueue(zoneQueue)
	getMyOrder := orderapp.NewGetMyOrder(customerRepo, orderRepo).WithZoneQueue(zoneQueue)
	routeAssignment := orderapp.NewRouteAssignment(orderRepo).WithZoneQueue(zoneQueue)
	advanceRouteStop := orderapp.NewAdvanceRouteStop(orderRepo)
	stopsBefore := orderapp.NewStopsBeforeForOrder(customerRepo, orderRepo)
	driverRouteStops := orderapp.NewDriverRouteStops(orderRepo)
	surchargeRepo := persistence.NewSurchargeRepository(pool)
	reincidence := orderapp.NewReincidence(surchargeRepo)
	recordStopOutcome := orderapp.NewRecordStopOutcome(orderRepo).WithZoneQueue(zoneQueue).
		WithSurcharges(surchargeRepo).
		WithReincidence(reincidence)
	log.Info("order use cases wired", "create", createOrder != nil, "review", reviewOrder != nil)

	// gRPC server para Customer y Order. La autorización por rol se aplica en
	// los casos de uso; el Gateway solo propaga la identidad del JWT.
	grpcServer := grpc.NewServer()
	ordersv1.RegisterOrderServiceServer(grpcServer, grpcadapter.NewOrderServer(grpcadapter.OrderUseCases{
		Create:           createOrder,
		ListMine:         listMyOrders,
		Exceptions:       listExceptions,
		ListAll:          listAllOrders,
		Review:           reviewOrder,
		Notifications:    listMyNotifications,
		Cancel:           cancelOrder,
		GetMine:          getMyOrder,
		RouteAssignment:  routeAssignment,
		AdvanceStop:      advanceRouteStop,
		StopsBefore:      stopsBefore,
		DriverRouteStops: driverRouteStops,
		RecordOutcome:    recordStopOutcome,
		Surcharges:       orderapp.NewResolveSurchargeVerification(surchargeRepo).WithReincidence(reincidence),
		Recidivism:       orderapp.NewApplyRecidivismCharge(persistence.NewRecidivismRepository(pool)),
		Pay: orderapp.NewPayBalance(customerRepo, payments.NewSimulated(time.Now),
			persistence.NewPaymentAttemptRepository(pool), persistence.NewBalanceRepository(pool)),
	}).WithRouteCapacity(zoneClassifier))
	// Catálogo de productos (ADR 0009): el operador administra, el cliente lee lo activo.
	catalogv1.RegisterCatalogServiceServer(grpcServer, grpcadapter.NewCatalogServer(
		catalogcase.NewService(catalogStore, time.Now),
	))
	customerv1.RegisterCustomerServiceServer(grpcServer, grpcadapter.NewCustomerServer(grpcadapter.UseCases{
		Create:          createCustomer,
		GetByID:         getCustomerByID,
		List:            listCustomers,
		GetByUserID:     getByUserID,
		UpdateStatus:    updateCustomerStatus,
		CanCreateOrders: canCreateOrders,
	}))

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           httpx.Handler(pool.Ping),
	}

	// ADR 0006, section 5: block the accounts whose unpaid balance is older than two weeks. The first pass
	// runs at start so a restart never leaves a late account open; a failure is logged and the next pass retries.
	go runBalanceSweep(ctx, log, orderapp.NewBlockOverdueBalances(persistence.NewBalanceRepository(pool)), cfg.BalanceSweepInterval)

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

// runBalanceSweep blocks overdue balances now and then on every tick until ctx ends.
func runBalanceSweep(ctx context.Context, log *slog.Logger, sweep *orderapp.BlockOverdueBalances, every time.Duration) {
	run := func() {
		blocked, err := sweep.Run(ctx)
		switch {
		case err != nil:
			log.Error("balance sweep failed; the next pass retries", "error", err)
		case blocked > 0:
			log.Warn("accounts blocked for an unpaid balance", "count", blocked)
		}
	}
	run()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
