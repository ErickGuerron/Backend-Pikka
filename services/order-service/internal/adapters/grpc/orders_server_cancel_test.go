package grpc

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// cancelOrderRepo guarda pedidos en memoria y aplica el mismo criterio de propiedad que la consulta SQL.
type cancelOrderRepo struct {
	ports.OrderRepository
	mu     sync.Mutex
	orders map[string]domain.Order
}

func (r *cancelOrderRepo) CancelForCustomer(_ context.Context, orderID string, customerID uuid.UUID, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.orders[orderID]
	if !ok || current.CustomerID != customerID.String() {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	next, err := decide(current)
	if err != nil {
		return domain.Order{}, err
	}
	r.orders[orderID] = next
	return next, nil
}

type cancelFixture struct {
	server   *OrderServer
	customer uuid.UUID
	order    string
}

func newCancelFixture(t *testing.T, status domain.Status) cancelFixture {
	t.Helper()
	customers := newFakeCustomerRepository()
	customerUser := uuid.New()
	customer, err := customers.Create(context.Background(), domain.Customer{UserID: customerUser})
	require.NoError(t, err)

	orderID := uuid.NewString()
	repo := &cancelOrderRepo{orders: map[string]domain.Order{
		orderID: {ID: orderID, CustomerID: customer.ID.String(), Status: status},
	}}
	server := NewOrderServer(OrderUseCases{
		Cancel: orderapp.NewCancelOrder(customers, repo),
	})
	return cancelFixture{server: server, customer: customerUser, order: orderID}
}

func identityCtx(userID uuid.UUID, role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataUserID, userID.String(),
		metadataUserRole, role,
	))
}

func TestCancelOrder_CustomerCancelsOwnOrder_ReturnsCancelled(t *testing.T) {
	f := newCancelFixture(t, domain.StatusRouteAssigned)

	res, err := f.server.CancelOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.CancelOrderRequest{Id: f.order})

	require.NoError(t, err)
	assert.Equal(t, f.order, res.GetOrder().GetId())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_CANCELLED, res.GetOrder().GetStatus())
}

func TestCancelOrder_ForeignOrder_ReturnsNotFound(t *testing.T) {
	f := newCancelFixture(t, domain.StatusPending)
	stranger := uuid.New()
	_, err := f.server.CancelOrder(identityCtx(stranger, "CUSTOMER"), &ordersv1.CancelOrderRequest{Id: f.order})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCancelOrder_OperatorRole_ReturnsPermissionDenied(t *testing.T) {
	f := newCancelFixture(t, domain.StatusPending)

	_, err := f.server.CancelOrder(identityCtx(f.customer, "OPERATOR"), &ordersv1.CancelOrderRequest{Id: f.order})

	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestCancelOrder_AfterReview_ReturnsAborted(t *testing.T) {
	f := newCancelFixture(t, domain.StatusPickedUp)

	_, err := f.server.CancelOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.CancelOrderRequest{Id: f.order})

	assert.Equal(t, codes.Aborted, status.Code(err))
}

func TestCancelOrder_InvalidUUID_ReturnsNotFound(t *testing.T) {
	f := newCancelFixture(t, domain.StatusPending)

	_, err := f.server.CancelOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.CancelOrderRequest{Id: "no-es-uuid"})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCancelOrder_MissingIdentity_ReturnsUnauthenticated(t *testing.T) {
	f := newCancelFixture(t, domain.StatusPending)

	_, err := f.server.CancelOrder(context.Background(), &ordersv1.CancelOrderRequest{Id: f.order})

	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCancelOrder_NotWired_ReturnsUnimplemented(t *testing.T) {
	server := NewOrderServer(OrderUseCases{})

	_, err := server.CancelOrder(identityCtx(uuid.New(), "CUSTOMER"), &ordersv1.CancelOrderRequest{Id: uuid.NewString()})

	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
