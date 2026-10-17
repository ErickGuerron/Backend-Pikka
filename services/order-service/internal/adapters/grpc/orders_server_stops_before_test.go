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

// stopsBeforeServerRepo aplica el mismo criterio de propiedad que la consulta SQL: un pedido
// ajeno o inexistente responde ErrOrderNotFound. Devuelve un conteo fijo para el pedido propio.
type stopsBeforeServerRepo struct {
	ports.RouteStopRepository
	mu    sync.Mutex
	owner map[string]uuid.UUID
	count int
	calls int
}

func (r *stopsBeforeServerRepo) CountStopsBefore(_ context.Context, orderID string, customerID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.owner[orderID] != customerID {
		return 0, domain.ErrOrderNotFound
	}
	return r.count, nil
}

type stopsBeforeFixture struct {
	server   *OrderServer
	repo     *stopsBeforeServerRepo
	customer uuid.UUID
	orderID  string
}

func newStopsBeforeFixture(t *testing.T) stopsBeforeFixture {
	t.Helper()
	customers := newFakeCustomerRepository()
	customerUser := uuid.New()
	customer, err := customers.Create(context.Background(), domain.Customer{UserID: customerUser})
	require.NoError(t, err)

	orderID := uuid.NewString()
	repo := &stopsBeforeServerRepo{owner: map[string]uuid.UUID{orderID: customer.ID}, count: 2}
	server := NewOrderServer(OrderUseCases{
		StopsBefore: orderapp.NewStopsBeforeForOrder(customers, repo),
	})
	return stopsBeforeFixture{server: server, repo: repo, customer: customerUser, orderID: orderID}
}

func TestStopsBeforeForOrder_CustomerGetsCountForOwnOrder(t *testing.T) {
	f := newStopsBeforeFixture(t)

	res, err := f.server.StopsBeforeForOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.StopsBeforeForOrderRequest{OrderId: f.orderID})

	require.NoError(t, err)
	assert.Equal(t, int32(2), res.GetStopsBefore())
}

func TestStopsBeforeForOrder_ForeignOrder_ReturnsNotFound(t *testing.T) {
	f := newStopsBeforeFixture(t)

	_, err := f.server.StopsBeforeForOrder(identityCtx(uuid.New(), "CUSTOMER"), &ordersv1.StopsBeforeForOrderRequest{OrderId: f.orderID})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestStopsBeforeForOrder_UnknownOrder_ReturnsNotFound(t *testing.T) {
	f := newStopsBeforeFixture(t)

	_, err := f.server.StopsBeforeForOrder(identityCtx(f.customer, "CUSTOMER"), &ordersv1.StopsBeforeForOrderRequest{OrderId: uuid.NewString()})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestStopsBeforeForOrder_NonCustomerRole_ReturnsPermissionDeniedWithoutRepository(t *testing.T) {
	for _, role := range []string{"OPERATOR", "DRIVER", "ADMIN"} {
		f := newStopsBeforeFixture(t)

		_, err := f.server.StopsBeforeForOrder(identityCtx(f.customer, role), &ordersv1.StopsBeforeForOrderRequest{OrderId: f.orderID})

		assert.Equal(t, codes.PermissionDenied, status.Code(err), role)
		assert.Zero(t, f.repo.calls, role)
	}
}

func TestStopsBeforeForOrder_MissingIdentity_ReturnsUnauthenticated(t *testing.T) {
	f := newStopsBeforeFixture(t)

	_, err := f.server.StopsBeforeForOrder(context.Background(), &ordersv1.StopsBeforeForOrderRequest{OrderId: f.orderID})

	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestStopsBeforeForOrder_NotWired_ReturnsUnimplemented(t *testing.T) {
	server := NewOrderServer(OrderUseCases{})

	_, err := server.StopsBeforeForOrder(identityCtx(uuid.New(), "CUSTOMER"), &ordersv1.StopsBeforeForOrderRequest{OrderId: uuid.NewString()})

	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// La respuesta solo lleva el conteo: ningún id de pedido ajeno puede viajar en ella.
func TestStopsBeforeForOrder_ResponseCarriesOnlyStopsBefore(t *testing.T) {
	res := &ordersv1.StopsBeforeForOrderResponse{StopsBefore: 3}

	fields := res.ProtoReflect().Descriptor().Fields()

	require.Equal(t, 1, fields.Len(), "the response must have a single field")
	assert.Equal(t, "stops_before", string(fields.Get(0).Name()))
	assert.Nil(t, fields.ByName("order_id"), "the response must not expose order ids")
}
