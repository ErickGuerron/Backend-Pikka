package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
)

// fakeStops es la fuente de paradas de prueba: devuelve la parada o un error fijo.
type fakeStops struct {
	stop domain.Stop
	err  error
}

func (f fakeStops) Stop(_ context.Context, _, _ string) (domain.Stop, error) {
	return f.stop, f.err
}

var (
	stopLoc  = domain.GeoPoint{Latitude: -0.1807, Longitude: -78.4678}
	stopInfo = domain.Stop{Location: stopLoc, Window: domain.ArrivalWindow{Start: t0, End: t0.Add(3 * time.Hour)}}
)

// serveDeliveries arranca el servidor con RegisterDelivery activo. stops nil deja la fuente sin configurar.
func serveDeliveries(t *testing.T, repo *memory.DriverRepository, stops grpcadapter.StopSource) driversv1.DriverServiceClient {
	t.Helper()
	ctx := context.Background()
	svc := driver.NewService(repo, func() time.Time { return t0 })
	deliveries := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), domain.DefaultArrivalRadiusMeters)
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	driversv1.RegisterDriverServiceServer(srv, grpcadapter.NewDriverServer(svc).WithDeliveries(deliveries, stops))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return driversv1.NewDriverServiceClient(conn)
}

// seedOnRoute deja al repartidor ON_ROUTE en routeID.
func seedOnRoute(t *testing.T, repo *memory.DriverRepository, id, routeID string) {
	t.Helper()
	ctx := context.Background()
	d, err := domain.NewDriver(id, domaintest.Input("user-"+id), t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, d))
	stored, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.NoError(t, stored.Reserve(routeID, t0))
	require.NoError(t, stored.StartRoute(routeID, t0))
	require.NoError(t, repo.Save(ctx, stored, stored.Version-2))
}

func deliveryReq(outcome, reason string) *driversv1.RegisterDeliveryRequest {
	return &driversv1.RegisterDeliveryRequest{
		DriverId: "d1", RouteId: "route-1", StopId: "stop-1",
		Outcome: outcome, Reason: reason,
		RecordedAt: t0.Add(time.Hour).Format(time.RFC3339),
		Latitude:   stopLoc.Latitude, Longitude: stopLoc.Longitude,
	}
}

func TestGetDriverByUserIDDevuelveElResumen(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedDriver(t, repo, "d1")
	client := serveDeliveries(t, repo, nil)

	res, err := client.GetDriverByUserID(context.Background(), &driversv1.GetDriverByUserIDRequest{UserId: "user-d1"})
	require.NoError(t, err)
	assert.Equal(t, "d1", res.GetDriver().GetId())
	assert.Equal(t, "user-d1", res.GetDriver().GetUserId())
}

func TestGetDriverByUserIDIncluyeLaRutaReservada(t *testing.T) {
	client, repo := setup(t)
	d := seedDriver(t, repo, "d1")
	stored := d.Version
	require.NoError(t, d.Reserve("route-7", t0))
	require.NoError(t, repo.Save(context.Background(), d, stored))

	res, err := client.GetDriverByUserID(context.Background(), &driversv1.GetDriverByUserIDRequest{UserId: "user-d1"})
	require.NoError(t, err)
	assert.Equal(t, "route-7", res.GetDriver().GetRouteId())
}

func TestGetDriverByUserIDCodigosDeError(t *testing.T) {
	repo := memory.NewDriverRepository()
	client := serveDeliveries(t, repo, nil)

	_, err := client.GetDriverByUserID(context.Background(), &driversv1.GetDriverByUserIDRequest{UserId: "  "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = client.GetDriverByUserID(context.Background(), &driversv1.GetDriverByUserIDRequest{UserId: "user-x"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestRegisterDeliveryRegistraConLaParadaDelRouting(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, fakeStops{stop: stopInfo})

	res, err := client.RegisterDelivery(context.Background(), deliveryReq("DELIVERED", ""))
	require.NoError(t, err)
	assert.Equal(t, "DELIVERED", res.GetOutcome())
	assert.True(t, res.GetWithinRadius())
	assert.True(t, res.GetWithinWindow())
}

func TestRegisterDeliveryNoSeAceptaDosVeces(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, fakeStops{stop: stopInfo})

	_, err := client.RegisterDelivery(context.Background(), deliveryReq("DELIVERED", ""))
	require.NoError(t, err)
	_, err = client.RegisterDelivery(context.Background(), deliveryReq("DELIVERED", ""))
	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestRegisterDeliverySinFuenteDeParadasEsUnimplemented(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, nil)

	_, err := client.RegisterDelivery(context.Background(), deliveryReq("DELIVERED", ""))
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestRegisterDeliveryResultadoInvalido(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, fakeStops{stop: stopInfo})

	_, err := client.RegisterDelivery(context.Background(), deliveryReq("LOST", ""))
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRegisterDeliveryMotivoObligatorioEnNoEntregado(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, fakeStops{stop: stopInfo})

	_, err := client.RegisterDelivery(context.Background(), deliveryReq("NOT_DELIVERED", ""))
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRegisterDeliveryFechaInvalida(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, fakeStops{stop: stopInfo})
	req := deliveryReq("DELIVERED", "")
	req.RecordedAt = "ayer"

	_, err := client.RegisterDelivery(context.Background(), req)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRegisterDeliveryRepartidorSinEsaRuta(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedDriver(t, repo, "d1")
	client := serveDeliveries(t, repo, fakeStops{stop: stopInfo})

	_, err := client.RegisterDelivery(context.Background(), deliveryReq("DELIVERED", ""))
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestRegisterDeliveryParadaDesconocidaEnElRouting(t *testing.T) {
	repo := memory.NewDriverRepository()
	seedOnRoute(t, repo, "d1", "route-1")
	client := serveDeliveries(t, repo, fakeStops{err: ports.ErrStopNotFound})

	_, err := client.RegisterDelivery(context.Background(), deliveryReq("DELIVERED", ""))
	assert.Equal(t, codes.NotFound, status.Code(err))
}
