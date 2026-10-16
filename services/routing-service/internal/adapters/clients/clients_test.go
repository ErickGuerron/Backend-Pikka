package clients

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
)

// fakeOrders records what reached Order Service: the RPC input and the identity metadata.
type fakeOrders struct {
	ordersv1.UnimplementedOrderServiceServer
	pending  []*ordersv1.Order
	gotDate  string
	gotRoute string
	gotIDs   []string
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

func TestOrderGatewayAssignToRouteSendsRouteAndIDs(t *testing.T) {
	fake := &fakeOrders{}
	conn := dial(t, func(s *grpc.Server) { ordersv1.RegisterOrderServiceServer(s, fake) })
	gw := NewOrderGateway(ordersv1.NewOrderServiceClient(conn))

	if err := gw.AssignToRoute(operatorCtx(), "2026-10-15/zone-a#1", []string{"o1", "o2"}); err != nil {
		t.Fatalf("AssignToRoute: %v", err)
	}
	if fake.gotRoute != "2026-10-15/zone-a#1" || len(fake.gotIDs) != 2 || fake.gotIDs[0] != "o1" {
		t.Fatalf("sent route=%q ids=%v", fake.gotRoute, fake.gotIDs)
	}
	if fake.gotMD.Get(metadataUserID)[0] == "" {
		t.Fatalf("identity not forwarded: %v", fake.gotMD)
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
