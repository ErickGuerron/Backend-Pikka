package clients

import (
	"context"
	"net"
	"slices"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// fakeOrders records what reached Order Service: the RPC input and the identity metadata.
type fakeOrders struct {
	ordersv1.UnimplementedOrderServiceServer
	pending  []*ordersv1.Order
	gotDate  string
	gotRoute string
	gotIDs   []string
	gotStops []*ordersv1.OrderStop
	gotMD    metadata.MD
}

func (f *fakeOrders) GetPendingOrders(ctx context.Context, req *ordersv1.GetPendingOrdersRequest) (*ordersv1.GetPendingOrdersResponse, error) {
	f.gotDate = req.GetDeliveryDate()
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	return &ordersv1.GetPendingOrdersResponse{Orders: f.pending}, nil
}

func (f *fakeOrders) AssignOrdersToRoute(ctx context.Context, req *ordersv1.AssignOrdersToRouteRequest) (*ordersv1.AssignOrdersToRouteResponse, error) {
	f.gotRoute = req.GetRouteId()
	f.gotIDs = req.GetOrderIds()
	f.gotStops = req.GetStops()
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	return &ordersv1.AssignOrdersToRouteResponse{}, nil
}

// fakeZones records the zone asked for and the identity metadata.
type fakeZones struct {
	zonesv1.UnimplementedZoneServiceServer
	maxPerRoute int32
	gotZone     string
	gotMD       metadata.MD
}

func (f *fakeZones) GetZone(ctx context.Context, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	f.gotZone = req.GetId()
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	return &zonesv1.GetZoneResponse{Zone: &zonesv1.Zone{
		Id:       req.GetId(),
		Settings: &zonesv1.ZoneSettings{MaxOrdersPerRoute: f.maxPerRoute},
	}}, nil
}

// dial starts the fake servers on an in-memory listener and returns one connection.
func dial(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// operatorCtx simulates the identity the Gateway propagates for an operator.
func operatorCtx() context.Context {
	md := metadata.Pairs(metadataUserID, "11111111-1111-1111-1111-111111111111", metadataUserRole, "OPERATOR")
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestOrderGatewayPendingByDateMapsOrdersAndForwardsIdentity(t *testing.T) {
	fake := &fakeOrders{pending: []*ordersv1.Order{
		{Id: "o1", ZoneId: "zone-a", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING},
		{Id: "o2", ZoneId: "zone-b", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING},
	}}
	conn := dial(t, func(s *grpc.Server) { ordersv1.RegisterOrderServiceServer(s, fake) })
	gw := NewOrderGateway(ordersv1.NewOrderServiceClient(conn))

	got, err := gw.PendingByDate(operatorCtx(), "2026-10-15")
	if err != nil {
		t.Fatalf("PendingByDate: %v", err)
	}
	if fake.gotDate != "2026-10-15" {
		t.Fatalf("date sent = %q", fake.gotDate)
	}
	if len(got) != 2 || got[0].OrderID != "o1" || got[0].ZoneID != "zone-a" || got[1].ZoneID != "zone-b" {
		t.Fatalf("mapped orders = %+v", got)
	}
	if fake.gotMD.Get(metadataUserRole)[0] != "OPERATOR" {
		t.Fatalf("role not forwarded: %v", fake.gotMD)
	}
}

func TestOrderGatewayAssignToRouteSendsStopsWithSequence(t *testing.T) {
	fake := &fakeOrders{}
	conn := dial(t, func(s *grpc.Server) { ordersv1.RegisterOrderServiceServer(s, fake) })
	gw := NewOrderGateway(ordersv1.NewOrderServiceClient(conn))

	stops := []domain.RouteStop{
		{OrderID: "o2", ZoneID: "zone-a", Sequence: 1},
		{OrderID: "o1", ZoneID: "zone-a", Sequence: 2},
	}
	if err := gw.AssignToRoute(operatorCtx(), "2026-10-15/zone-a#1", stops); err != nil {
		t.Fatalf("AssignToRoute: %v", err)
	}
	if fake.gotRoute != "2026-10-15/zone-a#1" {
		t.Fatalf("sent route=%q", fake.gotRoute)
	}
	if len(fake.gotStops) != 2 {
		t.Fatalf("sent stops = %v, want 2 stops", fake.gotStops)
	}
	for i, want := range stops {
		got := fake.gotStops[i]
		if got.GetOrderId() != want.OrderID || got.GetSequence() != int32(want.Sequence) {
			t.Fatalf("stop %d = %s#%d, want %s#%d", i, got.GetOrderId(), got.GetSequence(), want.OrderID, want.Sequence)
		}
	}
	if len(fake.gotIDs) != 0 {
		t.Fatalf("legacy order_ids sent alongside stops: %v", fake.gotIDs)
	}
	if fake.gotMD.Get(metadataUserID)[0] == "" {
		t.Fatalf("identity not forwarded: %v", fake.gotMD)
	}
}

// fakeDrivers records the limit and the reservation sent to Driver Service.
type fakeDrivers struct {
	driversv1.UnimplementedDriverServiceServer
	available  []*driversv1.DriverSummary
	gotLimit   int32
	reserved   []*driversv1.ReserveDriverRequest
	reserveErr error
	released   []*driversv1.ReleaseDriverRequest
	releaseErr error
	gotMD      metadata.MD
}

func (f *fakeDrivers) ReleaseDriver(ctx context.Context, req *driversv1.ReleaseDriverRequest) (*driversv1.ReleaseDriverResponse, error) {
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	f.released = append(f.released, req)
	return &driversv1.ReleaseDriverResponse{}, nil
}

func TestDriverPoolReleaseSendsDriverAndRouteAndForwardsIdentity(t *testing.T) {
	fake := &fakeDrivers{}
	conn := dial(t, func(s *grpc.Server) { driversv1.RegisterDriverServiceServer(s, fake) })
	pool := NewDriverPool(driversv1.NewDriverServiceClient(conn))

	if err := pool.Release(operatorCtx(), "d1", "2026-10-15/zone-a#1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(fake.released) != 1 || fake.released[0].GetDriverId() != "d1" || fake.released[0].GetRouteId() != "2026-10-15/zone-a#1" {
		t.Fatalf("release sent = %+v", fake.released)
	}
	if fake.gotMD.Get(metadataUserRole)[0] != "OPERATOR" {
		t.Fatalf("role not forwarded: %v", fake.gotMD)
	}
}

func TestDriverPoolReleasePreservesDriverServiceErrorCode(t *testing.T) {
	fake := &fakeDrivers{releaseErr: status.Error(codes.FailedPrecondition, "reserved for another route")}
	conn := dial(t, func(s *grpc.Server) { driversv1.RegisterDriverServiceServer(s, fake) })
	pool := NewDriverPool(driversv1.NewDriverServiceClient(conn))

	err := pool.Release(operatorCtx(), "d1", "route-2")

	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %s, want FAILED_PRECONDITION", status.Code(err))
	}
}

func (f *fakeDrivers) FindAvailableDrivers(ctx context.Context, req *driversv1.FindAvailableDriversRequest) (*driversv1.FindAvailableDriversResponse, error) {
	f.gotLimit = req.GetLimit()
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	return &driversv1.FindAvailableDriversResponse{Drivers: f.available}, nil
}

func (f *fakeDrivers) ReserveDriver(ctx context.Context, req *driversv1.ReserveDriverRequest) (*driversv1.ReserveDriverResponse, error) {
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	if f.reserveErr != nil {
		return nil, f.reserveErr
	}
	f.reserved = append(f.reserved, req)
	return &driversv1.ReserveDriverResponse{Driver: &driversv1.Driver{Id: req.GetDriverId()}}, nil
}

func TestDriverPoolFindAvailableMapsIDsAndForwardsIdentity(t *testing.T) {
	fake := &fakeDrivers{available: []*driversv1.DriverSummary{{Id: "d1", UserId: "u1"}, {Id: "d2", UserId: "u2"}}}
	conn := dial(t, func(s *grpc.Server) { driversv1.RegisterDriverServiceServer(s, fake) })
	pool := NewDriverPool(driversv1.NewDriverServiceClient(conn))

	got, err := pool.FindAvailable(operatorCtx(), 2)
	if err != nil {
		t.Fatalf("FindAvailable: %v", err)
	}
	if fake.gotLimit != 2 {
		t.Fatalf("limit sent = %d, want 2", fake.gotLimit)
	}
	if !slices.Equal(got, []string{"d1", "d2"}) {
		t.Fatalf("driver ids = %v, want [d1 d2]", got)
	}
	if fake.gotMD.Get(metadataUserRole)[0] != "OPERATOR" {
		t.Fatalf("role not forwarded: %v", fake.gotMD)
	}
}

func TestDriverPoolReserveSendsDriverAndRoute(t *testing.T) {
	fake := &fakeDrivers{}
	conn := dial(t, func(s *grpc.Server) { driversv1.RegisterDriverServiceServer(s, fake) })
	pool := NewDriverPool(driversv1.NewDriverServiceClient(conn))

	if err := pool.Reserve(operatorCtx(), "d1", "2026-10-15/zone-a#1"); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if len(fake.reserved) != 1 || fake.reserved[0].GetDriverId() != "d1" || fake.reserved[0].GetRouteId() != "2026-10-15/zone-a#1" {
		t.Fatalf("reservation sent = %+v", fake.reserved)
	}
	if fake.gotMD.Get(metadataUserID)[0] == "" {
		t.Fatalf("identity not forwarded: %v", fake.gotMD)
	}
}

func TestDriverPoolReservePreservesDriverServiceErrorCode(t *testing.T) {
	fake := &fakeDrivers{reserveErr: status.Error(codes.FailedPrecondition, "driver already reserved")}
	conn := dial(t, func(s *grpc.Server) { driversv1.RegisterDriverServiceServer(s, fake) })
	pool := NewDriverPool(driversv1.NewDriverServiceClient(conn))

	err := pool.Reserve(operatorCtx(), "d1", "2026-10-15/zone-a#1")
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (err %v)", status.Code(err), err)
	}
}

func TestZoneCapacitiesReadsSettingsAndForwardsIdentity(t *testing.T) {
	fake := &fakeZones{maxPerRoute: 4}
	conn := dial(t, func(s *grpc.Server) { zonesv1.RegisterZoneServiceServer(s, fake) })
	zones := NewZoneCapacities(zonesv1.NewZoneServiceClient(conn))

	got, err := zones.MaxOrdersPerRoute(operatorCtx(), "zone-a")
	if err != nil {
		t.Fatalf("MaxOrdersPerRoute: %v", err)
	}
	if got != 4 || fake.gotZone != "zone-a" {
		t.Fatalf("capacity = %d zone = %q", got, fake.gotZone)
	}
	if fake.gotMD.Get(metadataUserRole)[0] != "OPERATOR" {
		t.Fatalf("role not forwarded: %v", fake.gotMD)
	}
}
