package grpc

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// getOrderRepo guarda pedidos en memoria y aplica el mismo criterio de propiedad que la consulta SQL.
type getOrderRepo struct {
	ports.OrderRepository
	mu     sync.Mutex
	orders map[string]domain.Order
}

func (r *getOrderRepo) GetForCustomer(_ context.Context, orderID string, customerID uuid.UUID) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	order, ok := r.orders[orderID]
	if !ok || order.CustomerID != customerID.String() {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

type getMyOrderFixture struct {
	server   *OrderServer
	customer uuid.UUID
	order    domain.Order
}

func newGetMyOrderFixture(t *testing.T) getMyOrderFixture {
	t.Helper()
	customers := newFakeCustomerRepository()
	customerUser := uuid.New()
	customer, err := customers.Create(context.Background(), domain.Customer{UserID: customerUser})
	require.NoError(t, err)

	order := domain.Order{
		ID:         uuid.NewString(),
		CustomerID: customer.ID.String(),
		RouteID:    uuid.NewString(),
		Status:     domain.StatusRouteAssigned,
	}
	repo := &getOrderRepo{orders: map[string]domain.Order{order.ID: order}}
	server := NewOrderServer(OrderUseCases{
		GetMine: orderapp.NewGetMyOrder(customers, repo),
	})
	return getMyOrderFixture{server: server, customer: customerUser, order: order}
}

func TestGetMyOrder_CustomerReadsOwnOrder_ReturnsOrderWithRouteID(t *testing.T) {
	f := newGetMyOrderFixture(t)

	res, err := f.server.GetMyOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.GetMyOrderRequest{OrderId: f.order.ID})

	require.NoError(t, err)
	assert.Equal(t, f.order.ID, res.GetOrder().GetId())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, res.GetOrder().GetStatus())
	assert.Equal(t, f.order.RouteID, res.GetOrder().GetRouteId())
}

func TestGetMyOrder_ForeignOrder_ReturnsNotFound(t *testing.T) {
	f := newGetMyOrderFixture(t)

	_, err := f.server.GetMyOrder(identityCtx(uuid.New(), "CUSTOMER"), &ordersv1.GetMyOrderRequest{OrderId: f.order.ID})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetMyOrder_UnknownOrder_ReturnsNotFound(t *testing.T) {
	f := newGetMyOrderFixture(t)

	_, err := f.server.GetMyOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.GetMyOrderRequest{OrderId: uuid.NewString()})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetMyOrder_OperatorRole_ReturnsPermissionDenied(t *testing.T) {
	f := newGetMyOrderFixture(t)

	_, err := f.server.GetMyOrder(identityCtx(f.customer, "OPERATOR"), &ordersv1.GetMyOrderRequest{OrderId: f.order.ID})

	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetMyOrder_MissingIdentity_ReturnsUnauthenticated(t *testing.T) {
	f := newGetMyOrderFixture(t)

	_, err := f.server.GetMyOrder(context.Background(), &ordersv1.GetMyOrderRequest{OrderId: f.order.ID})

	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGetMyOrder_NotWired_ReturnsUnimplemented(t *testing.T) {
	server := NewOrderServer(OrderUseCases{})

	_, err := server.GetMyOrder(identityCtx(uuid.New(), "CUSTOMER"), &ordersv1.GetMyOrderRequest{OrderId: uuid.NewString()})

	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
