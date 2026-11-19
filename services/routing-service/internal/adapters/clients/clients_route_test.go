package clients

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// listingOrders answers ListOrders honoring the status and day filters, and records the request.
type listingOrders struct {
	ordersv1.UnimplementedOrderServiceServer
	orders []*ordersv1.Order
	gotReq *ordersv1.ListOrdersRequest
	gotMD  metadata.MD
}

func (f *listingOrders) ListOrders(ctx context.Context, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	f.gotReq = req
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	var out []*ordersv1.Order
	for _, o := range f.orders {
		if req.GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED && o.GetStatus() != req.GetStatus() {
			continue
		}
		if req.GetDeliveryDate() != "" && o.GetDeliveryDate() != req.GetDeliveryDate() {
			continue
		}
		out = append(out, o)
	}
	return &ordersv1.ListOrdersResponse{Orders: out}, nil
}

func listingGateway(t *testing.T, fake *listingOrders) *OrderGateway {
	t.Helper()
	conn := dial(t, func(s *grpc.Server) { ordersv1.RegisterOrderServiceServer(s, fake) })
	return NewOrderGateway(ordersv1.NewOrderServiceClient(conn))
}

func TestOrderGatewayFindPendingLooksAmongPendingOrdersAndForwardsIdentity(t *testing.T) {
	fake := &listingOrders{orders: []*ordersv1.Order{
		{Id: "n1", ZoneId: "A", Address: "Calle 1", DeliveryDate: "2026-10-15", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING},
		{Id: "x1", ZoneId: "A", Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED},
	}}
	gw := listingGateway(t, fake)

	got, found, err := gw.FindPending(operatorCtx(), "n1")
	if err != nil || !found {
		t.Fatalf("FindPending(n1) = %+v, %v, %v", got, found, err)
	}
	if got.ZoneID != "A" || got.Address != "Calle 1" || got.DeliveryDate != "2026-10-15" {
		t.Fatalf("mapped order = %+v", got)
	}
	if fake.gotReq.GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_PENDING || fake.gotMD.Get(metadataUserRole)[0] != "OPERATOR" {
		t.Fatalf("request = %v, md = %v", fake.gotReq, fake.gotMD)
	}

	if _, found, _ := gw.FindPending(operatorCtx(), "x1"); found {
		t.Fatal("an order that is not PENDING must not be found")
	}
}

func TestOrderGatewayRouteOrdersKeepsTheRouteWithoutCancelledOnesAndFlagsOpenStops(t *testing.T) {
	const day = "2026-10-15"
	fake := &listingOrders{orders: []*ordersv1.Order{
		{Id: "a", RouteId: "r1", ZoneId: "A", StopSequence: 1, DeliveryDate: day, Status: ordersv1.OrderStatus_ORDER_STATUS_DELIVERED},
		{Id: "b", RouteId: "r1", ZoneId: "A", StopSequence: 2, DeliveryDate: day, Status: ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT},
		{Id: "c", RouteId: "r1", ZoneId: "A", StopSequence: 3, DeliveryDate: day, Status: ordersv1.OrderStatus_ORDER_STATUS_CANCELLED},
		{Id: "d", RouteId: "r2", ZoneId: "A", StopSequence: 1, DeliveryDate: day, Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED},
		{Id: "e", RouteId: "r1", ZoneId: "A", StopSequence: 1, DeliveryDate: "2026-10-16", Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED},
	}}
	gw := listingGateway(t, fake)

	got, err := gw.RouteOrders(operatorCtx(), day, "r1")
	if err != nil {
		t.Fatalf("RouteOrders: %v", err)
	}
	if len(got) != 2 || got[0].OrderID != "a" || got[0].Open || got[1].OrderID != "b" || !got[1].Open || got[1].Sequence != 2 {
		t.Fatalf("route orders = %+v, want a (done) and b (open, seq 2) only", got)
	}
	if fake.gotReq.GetDeliveryDate() != day {
		t.Fatalf("day asked = %q", fake.gotReq.GetDeliveryDate())
	}
}
