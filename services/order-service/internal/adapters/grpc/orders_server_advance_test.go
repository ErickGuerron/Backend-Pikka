package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// sequenceRouteRepo calls decide per id (as the real repository does) and records the sequence each order got.
type sequenceRouteRepo struct {
	ports.OrderRepository
	sequences map[string]int
	calls     int
}

func (r *sequenceRouteRepo) ListPendingByDate(_ context.Context, _ string) ([]domain.Order, error) {
	return nil, nil
}

func (r *sequenceRouteRepo) AssignToRoute(_ context.Context, ids []string, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	r.calls++
	r.sequences = map[string]int{}
	out := make([]domain.Order, 0, len(ids))
	for _, id := range ids {
		next, err := decide(domain.Order{ID: id, Status: domain.StatusPending})
		if err != nil {
			return nil, err
		}
		r.sequences[id] = next.StopSequence
		out = append(out, next)
	}
	return out, nil
}

func sequenceServer(repo *sequenceRouteRepo) *OrderServer {
	return NewOrderServer(OrderUseCases{RouteAssignment: orderapp.NewRouteAssignment(repo)})
}

func TestAssignOrdersToRouteMapsStopSequences(t *testing.T) {
	first, second := uuid.NewString(), uuid.NewString()
	repo := &sequenceRouteRepo{}

	_, err := sequenceServer(repo).AssignOrdersToRoute(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.AssignOrdersToRouteRequest{
		RouteId: "zone-1#1",
		Stops: []*ordersv1.OrderStop{
			{OrderId: first, Sequence: 1},
			{OrderId: second, Sequence: 2},
		},
	})

	if err != nil {
		t.Fatalf("AssignOrdersToRoute: %v", err)
	}
	if repo.sequences[first] != 1 || repo.sequences[second] != 2 {
		t.Fatalf("sequences = %v, want first=1 second=2", repo.sequences)
	}
}

func TestAssignOrdersToRouteStopsWinOverOrderIDs(t *testing.T) {
	stopID, legacyID := uuid.NewString(), uuid.NewString()
	repo := &sequenceRouteRepo{}

	_, err := sequenceServer(repo).AssignOrdersToRoute(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.AssignOrdersToRouteRequest{
		RouteId:  "zone-1#1",
		OrderIds: []string{legacyID},
		Stops:    []*ordersv1.OrderStop{{OrderId: stopID, Sequence: 1}},
	})

	if err != nil {
		t.Fatalf("AssignOrdersToRoute: %v", err)
	}
	if _, ok := repo.sequences[legacyID]; ok || repo.sequences[stopID] != 1 {
		t.Fatalf("sequences = %v, want only the stop order", repo.sequences)
	}
}

func TestAssignOrdersToRouteInvalidStopSequenceIsInvalidArgument(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	cases := map[string][]*ordersv1.OrderStop{
		"zero":      {{OrderId: a, Sequence: 0}},
		"negative":  {{OrderId: a, Sequence: -1}},
		"duplicate": {{OrderId: a, Sequence: 1}, {OrderId: b, Sequence: 1}},
	}
	for name, stops := range cases {
		repo := &sequenceRouteRepo{}

		_, err := sequenceServer(repo).AssignOrdersToRoute(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.AssignOrdersToRouteRequest{
			RouteId: "zone-1#1", Stops: stops,
		})

		assertCode(t, err, codes.InvalidArgument)
		if repo.calls != 0 {
			t.Fatalf("%s: reached the repository", name)
		}
	}
}

// grpcAdvanceRepo applies the SQL rule (WHERE id AND route_id) to the orders it holds.
type grpcAdvanceRepo struct {
	ports.OrderRepository
	orders map[string]domain.Order
}

func (r *grpcAdvanceRepo) AdvanceRouteStop(_ context.Context, orderID, routeID string, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	order, ok := r.orders[orderID]
	if !ok || order.RouteID != routeID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	next, err := decide(order)
	if err != nil {
		return domain.Order{}, err
	}
	r.orders[orderID] = next
	return next, nil
}

func advanceServer(status domain.Status) (*OrderServer, string) {
	id := uuid.NewString()
	return advanceServerFor(id, status), id
}

// advanceServerFor seeds one order with the given id, on route zone-1#1, in the given status.
func advanceServerFor(id string, status domain.Status) *OrderServer {
	repo := &grpcAdvanceRepo{orders: map[string]domain.Order{
		id: {ID: id, RouteID: "zone-1#1", Status: status},
	}}
	return NewOrderServer(OrderUseCases{AdvanceStop: orderapp.NewAdvanceRouteStop(repo)})
}

func advanceReq(orderID string, target ordersv1.OrderStatus) *ordersv1.AdvanceRouteStopRequest {
	return &ordersv1.AdvanceRouteStopRequest{RouteId: "zone-1#1", OrderId: orderID, TargetStatus: target}
}

func driverCtx() context.Context {
	return routeIdentityCtx(orderapp.RoleDriver)
}

func TestAdvanceRouteStopReturnsUpdatedOrder(t *testing.T) {
	server, id := advanceServer(domain.StatusRouteAssigned)

	got, err := server.AdvanceRouteStop(driverCtx(), advanceReq(id, ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP))

	if err != nil {
		t.Fatalf("AdvanceRouteStop: %v", err)
	}
	if got.GetOrder().GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP {
		t.Fatalf("status = %s, want PICKED_UP", got.GetOrder().GetStatus())
	}
	if got.GetOrder().GetRouteId() != "zone-1#1" {
		t.Fatalf("route id = %q, want zone-1#1", got.GetOrder().GetRouteId())
	}
}

func TestAdvanceRouteStopErrorCodes(t *testing.T) {
	validID := uuid.NewString()
	cases := []struct {
		name string
		ctx  context.Context
		req  *ordersv1.AdvanceRouteStopRequest
		init domain.Status
		want codes.Code
	}{
		{"no identity", context.Background(), advanceReq(validID, ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP), domain.StatusRouteAssigned, codes.Unauthenticated},
		{"customer role", routeIdentityCtx(orderapp.RoleCustomer), advanceReq(validID, ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP), domain.StatusRouteAssigned, codes.PermissionDenied},
		{"blank route", driverCtx(), &ordersv1.AdvanceRouteStopRequest{OrderId: validID, TargetStatus: ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP}, domain.StatusRouteAssigned, codes.InvalidArgument},
		{"blank order", driverCtx(), &ordersv1.AdvanceRouteStopRequest{RouteId: "zone-1#1", TargetStatus: ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP}, domain.StatusRouteAssigned, codes.InvalidArgument},
		{"target DELIVERED", driverCtx(), advanceReq(validID, ordersv1.OrderStatus_ORDER_STATUS_DELIVERED), domain.StatusRouteAssigned, codes.InvalidArgument},
		{"target unspecified", driverCtx(), advanceReq(validID, ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED), domain.StatusRouteAssigned, codes.InvalidArgument},
		{"illegal transition", driverCtx(), advanceReq(validID, ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT), domain.StatusRouteAssigned, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		server := advanceServerFor(validID, tc.init)

		_, err := server.AdvanceRouteStop(tc.ctx, tc.req)

		if got := status.Code(err); got != tc.want {
			t.Fatalf("%s: code = %s, want %s (err %v)", tc.name, got, tc.want, err)
		}
	}
}

func TestAdvanceRouteStopOrderOnAnotherRouteIsNotFound(t *testing.T) {
	server, id := advanceServer(domain.StatusRouteAssigned)
	req := &ordersv1.AdvanceRouteStopRequest{RouteId: "zone-9#4", OrderId: id, TargetStatus: ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP}

	_, err := server.AdvanceRouteStop(driverCtx(), req)

	assertCode(t, err, codes.NotFound)
}
