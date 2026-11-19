package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// outcomeGRPCRepo serves one order to the outcome use case through the same port the advance path uses.
type outcomeGRPCRepo struct {
	ports.OrderRepository
	current domain.Order
	calls   int
}

func (r *outcomeGRPCRepo) AdvanceRouteStop(_ context.Context, _, _ string, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	r.calls++
	return decide(r.current)
}

func outcomeServer(repo *outcomeGRPCRepo) *OrderServer {
	return NewOrderServer(OrderUseCases{RecordOutcome: orderapp.NewRecordStopOutcome(repo)})
}

func TestRecordRouteStopOutcomeReturnsClosedOrder(t *testing.T) {
	id := uuid.NewString()
	repo := &outcomeGRPCRepo{current: domain.Order{ID: id, Status: domain.StatusInTransit, RouteID: "zone-1#1"}}

	res, err := outcomeServer(repo).RecordRouteStopOutcome(routeIdentityCtx(orderapp.RoleDriver), &ordersv1.RecordRouteStopOutcomeRequest{
		RouteId: "zone-1#1", OrderId: id, Outcome: ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED,
	})
	if err != nil {
		t.Fatalf("RecordRouteStopOutcome: %v", err)
	}
	if res.GetOrder().GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED {
		t.Fatalf("status = %s, want NOT_DELIVERED", res.GetOrder().GetStatus())
	}
}

func TestRecordRouteStopOutcomeMapsErrors(t *testing.T) {
	id := uuid.NewString()
	cases := []struct {
		name    string
		role    string
		outcome ordersv1.OrderStatus
		order   domain.Order
		want    codes.Code
	}{
		{"not driver", orderapp.RoleOperator, ordersv1.OrderStatus_ORDER_STATUS_DELIVERED, domain.Order{ID: id, Status: domain.StatusInTransit}, codes.PermissionDenied},
		{"picked up target", orderapp.RoleDriver, ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP, domain.Order{ID: id, Status: domain.StatusInTransit}, codes.InvalidArgument},
		{"unspecified target", orderapp.RoleDriver, ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, domain.Order{ID: id, Status: domain.StatusInTransit}, codes.InvalidArgument},
		{"not in transit", orderapp.RoleDriver, ordersv1.OrderStatus_ORDER_STATUS_DELIVERED, domain.Order{ID: id, Status: domain.StatusRouteAssigned}, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &outcomeGRPCRepo{current: tc.order}

			_, err := outcomeServer(repo).RecordRouteStopOutcome(routeIdentityCtx(tc.role), &ordersv1.RecordRouteStopOutcomeRequest{
				RouteId: "zone-1#1", OrderId: id, Outcome: tc.outcome,
			})
			assertCode(t, err, tc.want)
		})
	}
}

func TestRecordRouteStopOutcomeNotWiredIsUnimplemented(t *testing.T) {
	_, err := NewOrderServer(OrderUseCases{}).RecordRouteStopOutcome(routeIdentityCtx(orderapp.RoleDriver), &ordersv1.RecordRouteStopOutcomeRequest{})
	assertCode(t, err, codes.Unimplemented)
}
