package orders_test

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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/orders"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// fakeOrders es Order Service de prueba: guarda la metadata recibida y responde las paradas dadas.
type fakeOrders struct {
	ordersv1.UnimplementedOrderServiceServer
	stops    []*ordersv1.DriverRouteStop
	err      error
	gotMD    metadata.MD
	gotRoute string
}

func (f *fakeOrders) ListDriverRouteStops(ctx context.Context, req *ordersv1.ListDriverRouteStopsRequest) (*ordersv1.ListDriverRouteStopsResponse, error) {
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	f.gotRoute = req.GetRouteId()
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListDriverRouteStopsResponse{Stops: f.stops}, nil
}

// newSource arranca el fake sobre bufconn y devuelve la fuente de paradas contra el.
func newSource(t *testing.T, fake *fakeOrders) *orders.StopSource {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	ordersv1.RegisterOrderServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return orders.NewStopSource(ordersv1.NewOrderServiceClient(conn), 2*time.Second)
}

// driverCtx simula la identidad que el Gateway propaga al Driver Service.
func driverCtx() context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-user-id", "user-d1", "x-user-role", "DRIVER", "x-request-id", "req-1",
	))
}

var recordedAt = time.Date(2026, 10, 25, 10, 30, 0, 0, time.FixedZone("ECT", -5*3600))

func stopRow(id string) *ordersv1.DriverRouteStop {
	return &ordersv1.DriverRouteStop{
		OrderId: id, Sequence: 1, Address: "Av. Amazonas 100", Latitude: -0.18, Longitude: -78.48,
		WindowStart: "10:00", WindowEnd: "12:00", DeliveryDate: "2026-10-25",
	}
}

func TestStopSourceBuildsTheStopAndForwardsTheDriverIdentity(t *testing.T) {
	fake := &fakeOrders{stops: []*ordersv1.DriverRouteStop{stopRow("stop-1")}}
	source := newSource(t, fake)

	stop, err := source.Stop(driverCtx(), "route-1", "stop-1", recordedAt)

	require.NoError(t, err)
	assert.Equal(t, domain.GeoPoint{Latitude: -0.18, Longitude: -78.48}, stop.Location)
	assert.True(t, stop.Window.Start.Equal(time.Date(2026, 10, 25, 10, 0, 0, 0, recordedAt.Location())))
	assert.True(t, stop.Window.End.Equal(time.Date(2026, 10, 25, 12, 0, 0, 0, recordedAt.Location())))
	assert.Equal(t, "route-1", fake.gotRoute)
	assert.Equal(t, []string{"user-d1"}, fake.gotMD.Get("x-user-id"))
	assert.Equal(t, []string{"DRIVER"}, fake.gotMD.Get("x-user-role"))
}

func TestStopSourceParadaAjenaORecordadaEsStopNotFound(t *testing.T) {
	fake := &fakeOrders{stops: []*ordersv1.DriverRouteStop{stopRow("stop-2")}}
	source := newSource(t, fake)

	_, err := source.Stop(driverCtx(), "route-1", "stop-1", recordedAt)

	assert.ErrorIs(t, err, ports.ErrStopNotFound)
}

func TestStopSourceFallaOrderServiceNoSeTraduceAParada(t *testing.T) {
	fake := &fakeOrders{err: status.Error(codes.Unavailable, "down")}
	source := newSource(t, fake)

	_, err := source.Stop(driverCtx(), "route-1", "stop-1", recordedAt)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ports.ErrStopNotFound)
}
