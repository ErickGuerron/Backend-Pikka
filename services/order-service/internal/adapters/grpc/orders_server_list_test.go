package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type listFakeRepo struct {
	ports.OrderRepository
	filter ports.OrderFilter
	result []domain.Order
	calls  int
}

func (r *listFakeRepo) ListAll(_ context.Context, f ports.OrderFilter) ([]domain.Order, error) {
	r.calls++
	r.filter = f
	return r.result, nil
}

func listServer(repo *listFakeRepo) *OrderServer {
	return NewOrderServer(OrderUseCases{ListAll: orderapp.NewListOrders(repo)})
}

func TestListOrdersReturnsOrdersAndForwardsTheFilter(t *testing.T) {
	repo := &listFakeRepo{result: []domain.Order{{ID: "o-1", Status: domain.StatusPendingReview}}}

	got, err := listServer(repo).ListOrders(routeIdentityCtx(orderapp.RoleOperator), &ordersv1.ListOrdersRequest{
		DeliveryDate: " 2026-10-20 ", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW,
	})

	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if len(got.GetOrders()) != 1 || got.GetOrders()[0].GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW {
		t.Fatalf("orders = %+v", got.GetOrders())
	}
	if repo.filter.DeliveryDate != "2026-10-20" || repo.filter.Status != domain.StatusPendingReview {
		t.Fatalf("filter = %+v", repo.filter)
	}
}

func TestListOrdersWithoutFilterIsAnEmptyFilterAndAnEmptyListIsNotAnError(t *testing.T) {
	repo := &listFakeRepo{}

	got, err := listServer(repo).ListOrders(routeIdentityCtx(orderapp.RoleAdmin), &ordersv1.ListOrdersRequest{})

	if err != nil || len(got.GetOrders()) != 0 {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if repo.filter != (ports.OrderFilter{}) {
		t.Fatalf("filter = %+v, want empty", repo.filter)
	}
}

func TestListOrdersCodes(t *testing.T) {
	repo := &listFakeRepo{}
	server := listServer(repo)

	_, err := server.ListOrders(context.Background(), &ordersv1.ListOrdersRequest{})
	assertCode(t, err, codes.Unauthenticated)

	for _, role := range []string{orderapp.RoleCustomer, "DRIVER"} {
		_, err = server.ListOrders(routeIdentityCtx(role), &ordersv1.ListOrdersRequest{})
		assertCode(t, err, codes.PermissionDenied)
	}

	_, err = server.ListOrders(routeIdentityCtx(orderapp.RoleAdmin), &ordersv1.ListOrdersRequest{DeliveryDate: "20/10/2026"})
	assertCode(t, err, codes.InvalidArgument)

	_, err = server.ListOrders(routeIdentityCtx(orderapp.RoleAdmin), &ordersv1.ListOrdersRequest{Status: ordersv1.OrderStatus(99)})
	assertCode(t, err, codes.InvalidArgument)

	if repo.calls != 0 {
		t.Fatalf("rejected requests reached the repository (%d calls)", repo.calls)
	}
}

func TestListOrdersIsUnimplementedWhenNotWired(t *testing.T) {
	_, err := NewOrderServer(OrderUseCases{}).ListOrders(routeIdentityCtx(orderapp.RoleAdmin), &ordersv1.ListOrdersRequest{})
	assertCode(t, err, codes.Unimplemented)
}
