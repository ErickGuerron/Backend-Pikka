package clients

import (
	"context"
	"math"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// unavailableOrders responde UNAVAILABLE a las operaciones de pedidos.
type unavailableOrders struct {
	ordersv1.UnimplementedOrderServiceServer
}

func (unavailableOrders) GetPendingOrders(context.Context, *ordersv1.GetPendingOrdersRequest) (*ordersv1.GetPendingOrdersResponse, error) {
	return nil, status.Error(codes.Unavailable, "orders down")
}

func (unavailableOrders) AssignOrdersToRoute(context.Context, *ordersv1.AssignOrdersToRouteRequest) (*ordersv1.AssignOrdersToRouteResponse, error) {
	return nil, status.Error(codes.Unavailable, "orders down")
}

// unavailableDrivers responde UNAVAILABLE a la bÃºsqueda de conductores.
type unavailableDrivers struct {
	driversv1.UnimplementedDriverServiceServer
}

func (unavailableDrivers) FindAvailableDrivers(context.Context, *driversv1.FindAvailableDriversRequest) (*driversv1.FindAvailableDriversResponse, error) {
	return nil, status.Error(codes.Unavailable, "drivers down")
}

// covRingZones devuelve una zona con el anillo indicado, o un error si se configura.
type covRingZones struct {
	zonesv1.UnimplementedZoneServiceServer
	ring []*zonesv1.Coordinate
	err  error
}

func (z covRingZones) GetZone(_ context.Context, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	if z.err != nil {
		return nil, z.err
	}
	return &zonesv1.GetZoneResponse{Zone: &zonesv1.Zone{
		Id:       req.GetId(),
		Ring:     z.ring,
		Settings: &zonesv1.ZoneSettings{MaxOrdersPerRoute: 3},
	}}, nil
}

func TestOrderGatewayPassesServiceErrorsThrough(t *testing.T) {
	conn := dial(t, func(s *grpc.Server) { ordersv1.RegisterOrderServiceServer(s, unavailableOrders{}) })
	gw := NewOrderGateway(ordersv1.NewOrderServiceClient(conn))

	_, err := gw.PendingByDate(operatorCtx(), "2026-10-25")
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("PendingByDate code = %v, want UNAVAILABLE (err %v)", status.Code(err), err)
	}

	err = gw.AssignToRoute(operatorCtx(), "2026-10-25-a-1", []domain.RouteStop{{OrderID: "o1", Sequence: 1}})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("AssignToRoute code = %v, want UNAVAILABLE (err %v)", status.Code(err), err)
	}
}

func TestDriverPoolFindAvailablePassesServiceErrorsThrough(t *testing.T) {
	conn := dial(t, func(s *grpc.Server) { driversv1.RegisterDriverServiceServer(s, unavailableDrivers{}) })
	pool := NewDriverPool(driversv1.NewDriverServiceClient(conn))

	ids, err := pool.FindAvailable(operatorCtx(), 5)

	if ids != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("ids=%v err=%v, want no ids and UNAVAILABLE", ids, err)
	}
}

func TestZoneCapacitiesPassesServiceErrorsThrough(t *testing.T) {
	conn := dial(t, func(s *grpc.Server) {
		zonesv1.RegisterZoneServiceServer(s, covRingZones{err: status.Error(codes.NotFound, "zone not found")})
	})
	zones := NewZoneCapacities(zonesv1.NewZoneServiceClient(conn))

	capacity, err := zones.MaxOrdersPerRoute(operatorCtx(), "zone-x")
	if capacity != 0 || status.Code(err) != codes.NotFound {
		t.Fatalf("capacity=%d err=%v, want 0 and NOT_FOUND", capacity, err)
	}

	center, ok, err := zones.ZoneCenter(operatorCtx(), "zone-x")
	if ok || center != (domain.Point{}) || status.Code(err) != codes.NotFound {
		t.Fatalf("center=%+v ok=%v err=%v, want no center and NOT_FOUND", center, ok, err)
	}
}

func TestZoneCenterIsTheRingCentroid(t *testing.T) {
	ring := []*zonesv1.Coordinate{
		{Latitude: 0, Longitude: 0},
		{Latitude: 0, Longitude: 4},
		{Latitude: 2, Longitude: 4},
		{Latitude: 2, Longitude: 0},
		{Latitude: 0, Longitude: 0}, // punto de cierre repetido
	}
	conn := dial(t, func(s *grpc.Server) { zonesv1.RegisterZoneServiceServer(s, covRingZones{ring: ring}) })
	zones := NewZoneCapacities(zonesv1.NewZoneServiceClient(conn))

	center, ok, err := zones.ZoneCenter(operatorCtx(), "zone-a")

	if err != nil || !ok {
		t.Fatalf("center err=%v ok=%v", err, ok)
	}
	if math.Abs(center.Latitude-1) > 1e-9 || math.Abs(center.Longitude-2) > 1e-9 {
		t.Fatalf("center = %+v, want (1, 2)", center)
	}
}

func TestZoneWithoutRingHasNoCenter(t *testing.T) {
	conn := dial(t, func(s *grpc.Server) { zonesv1.RegisterZoneServiceServer(s, covRingZones{}) })
	zones := NewZoneCapacities(zonesv1.NewZoneServiceClient(conn))

	_, ok, err := zones.ZoneCenter(operatorCtx(), "zone-a")

	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want no center and no error", ok, err)
	}
}

func TestCallsWithoutIdentityStillReachTheService(t *testing.T) {
	fake := &fakeOrders{pending: []*ordersv1.Order{{Id: "o1", ZoneId: "zone-a"}}}
	conn := dial(t, func(s *grpc.Server) { ordersv1.RegisterOrderServiceServer(s, fake) })
	gw := NewOrderGateway(ordersv1.NewOrderServiceClient(conn))

	got, err := gw.PendingByDate(context.Background(), "2026-10-25")

	if err != nil {
		t.Fatalf("PendingByDate: %v", err)
	}
	if len(got) != 1 || len(fake.gotMD.Get(metadataUserRole)) != 0 {
		t.Fatalf("got=%+v md=%v, want the order and no forwarded identity", got, fake.gotMD)
	}
}
