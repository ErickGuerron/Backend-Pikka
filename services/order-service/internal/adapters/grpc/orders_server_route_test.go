package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type routeFakeRepo struct {
	pending []domain.Order
	assign  []domain.Order
	err     error
}

func (r *routeFakeRepo) ListPendingByDate(_ context.Context, _ string) ([]domain.Order, error) {
	return r.pending, r.err
}

func (r *routeFakeRepo) AssignToRoute(_ context.Context, _ []string, _ func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	return r.assign, r.err
}

func routeServer(repo *routeFakeRepo) *OrderServer {
	return NewOrderServer(OrderUseCases{RouteAssignment: orderapp.NewRouteAssignment(repo)})
}

// identityCtx simula la identidad que el Gateway propaga tras validar el JWT.
func routeIdentityCtx(role string) context.Context {
	md := metadata.Pairs(metadataUserID, uuid.NewString(), metadataUserRole, role)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestGetPendingOrdersMapsZoneForOperator(t *testing.T) {
	repo := &routeFakeRepo{pending: []domain.Order{{ID: uuid.NewString(), ZoneID: "zone-9", Status: domain.StatusPending}}}
	server := routeServer(repo)

	got, err := server.GetPendingOrders(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.GetPendingOrdersRequest{DeliveryDate: "2026-10-15"})
	if err != nil {
		t.Fatalf("GetPendingOrders: %v", err)
	}
	if len(got.GetOrders()) != 1 || got.GetOrders()[0].GetZoneId() != "zone-9" {
		t.Fatalf("orders = %+v, want zone_id zone-9", got.GetOrders())
	}
	if got.GetOrders()[0].GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_PENDING {
		t.Fatalf("status = %s, want PENDING", got.GetOrders()[0].GetStatus())
	}

	_, err = server.GetPendingOrders(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.GetPendingOrdersRequest{DeliveryDate: "15-10-2026"})
	assertCode(t, err, codes.InvalidArgument)
}

func TestRouteRPCsRequireIdentity(t *testing.T) {
	server := routeServer(&routeFakeRepo{})

	_, err := server.GetPendingOrders(context.Background(), &ordersv1.GetPendingOrdersRequest{DeliveryDate: "2026-10-15"})
	assertCode(t, err, codes.Unauthenticated)

	_, err = server.AssignOrdersToRoute(context.Background(), &ordersv1.AssignOrdersToRouteRequest{RouteId: "r1", OrderIds: []string{uuid.NewString()}})
	assertCode(t, err, codes.Unauthenticated)
}

func TestRouteRPCsDenyCustomers(t *testing.T) {
	server := routeServer(&routeFakeRepo{})
	ctx := routeIdentityCtx(orderapp.RoleCustomer)

	_, err := server.GetPendingOrders(ctx, &ordersv1.GetPendingOrdersRequest{DeliveryDate: "2026-10-15"})
	assertCode(t, err, codes.PermissionDenied)

	_, err = server.AssignOrdersToRoute(ctx, &ordersv1.AssignOrdersToRouteRequest{RouteId: "r1", OrderIds: []string{uuid.NewString()}})
	assertCode(t, err, codes.PermissionDenied)
}

func TestAssignOrdersToRouteErrorCodes(t *testing.T) {
	valid := uuid.NewString()
	ctx := routeIdentityCtx(orderapp.RoleOperator)
	cases := []struct {
		name string
		repo *routeFakeRepo
		req  *ordersv1.AssignOrdersToRouteRequest
		want codes.Code
	}{
		{"blank route", &routeFakeRepo{}, &ordersv1.AssignOrdersToRouteRequest{OrderIds: []string{valid}}, codes.InvalidArgument},
		{"duplicate ids", &routeFakeRepo{}, &ordersv1.AssignOrdersToRouteRequest{RouteId: "r1", OrderIds: []string{valid, valid}}, codes.InvalidArgument},
		{"not pending", &routeFakeRepo{err: domain.ErrOrderNotPending}, &ordersv1.AssignOrdersToRouteRequest{RouteId: "r1", OrderIds: []string{valid}}, codes.Aborted},
		{"unknown order", &routeFakeRepo{err: domain.ErrOrderNotFound}, &ordersv1.AssignOrdersToRouteRequest{RouteId: "r1", OrderIds: []string{valid}}, codes.NotFound},
	}
	for _, tc := range cases {
		_, err := routeServer(tc.repo).AssignOrdersToRoute(ctx, tc.req)
		if got := status.Code(err); got != tc.want {
			t.Fatalf("%s: code = %s, want %s (err %v)", tc.name, got, tc.want, err)
		}
	}
}

func TestAssignOrdersToRouteReturnsAssignedOrders(t *testing.T) {
	id := uuid.NewString()
	repo := &routeFakeRepo{assign: []domain.Order{{ID: id, ZoneID: "zone-9", Status: domain.StatusRouteAssigned}}}

	got, err := routeServer(repo).AssignOrdersToRoute(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.AssignOrdersToRouteRequest{
		RouteId: "zone-9#1", OrderIds: []string{id},
	})

	if err != nil {
		t.Fatalf("AssignOrdersToRoute: %v", err)
	}
	if len(got.GetOrders()) != 1 || got.GetOrders()[0].GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED {
		t.Fatalf("orders = %+v, want one ROUTE_ASSIGNED", got.GetOrders())
	}
}

func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s, want %s (err %v)", got, want, err)
	}
}
