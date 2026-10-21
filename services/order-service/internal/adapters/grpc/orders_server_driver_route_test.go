package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// driverRouteServerRepo devuelve las paradas preparadas para una ruta.
type driverRouteServerRepo struct {
	stops []domain.Order
	calls int
}

func (r *driverRouteServerRepo) ListDriverRouteStops(_ context.Context, _ string) ([]domain.Order, error) {
	r.calls++
	return r.stops, nil
}

func newDriverRouteServer(repo *driverRouteServerRepo) *OrderServer {
	return NewOrderServer(OrderUseCases{
		DriverRouteStops: orderapp.NewDriverRouteStops(repo),
	})
}

func TestListDriverRouteStops_DriverGetsStopsInSequenceWithoutCustomerData(t *testing.T) {
	window, err := domain.NewDeliveryWindow("10:00", "12:00")
	require.NoError(t, err)
	repo := &driverRouteServerRepo{stops: []domain.Order{
		{ID: "o-1", Address: "Av. Amazonas 100", StopSequence: 1, Status: domain.StatusRouteAssigned,
			Window: window, Latitude: -0.18, Longitude: -78.48,
			ContactPhone: "0999999999", CustomerID: "customer-secret"},
		{ID: "o-2", Address: "Calle Larga 5", StopSequence: 2, Status: domain.StatusPickedUp},
	}}
	server := newDriverRouteServer(repo)

	res, err := server.ListDriverRouteStops(identityCtx(uuid.New(), "DRIVER"), &ordersv1.ListDriverRouteStopsRequest{RouteId: "route-7"})

	require.NoError(t, err)
	require.Len(t, res.GetStops(), 2)
	first := res.GetStops()[0]
	assert.Equal(t, "o-1", first.GetOrderId())
	assert.Equal(t, int32(1), first.GetSequence())
	assert.Equal(t, "Av. Amazonas 100", first.GetAddress())
	assert.Equal(t, "10:00", first.GetWindowStart())
	assert.Equal(t, "12:00", first.GetWindowEnd())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, first.GetStatus())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP, res.GetStops()[1].GetStatus())
}

func TestListDriverRouteStops_NonDriverIsDeniedBeforeReading(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "ADMIN", "OPERATOR"} {
		repo := &driverRouteServerRepo{}
		server := newDriverRouteServer(repo)

		_, err := server.ListDriverRouteStops(identityCtx(uuid.New(), role), &ordersv1.ListDriverRouteStopsRequest{RouteId: "route-7"})

		require.Error(t, err, "role %s", role)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), "role %s", role)
		assert.Equal(t, 0, repo.calls, "role %s must not read the route", role)
	}
}

func TestListDriverRouteStops_BlankRouteIsInvalidArgument(t *testing.T) {
	repo := &driverRouteServerRepo{}
	server := newDriverRouteServer(repo)

	_, err := server.ListDriverRouteStops(identityCtx(uuid.New(), "DRIVER"), &ordersv1.ListDriverRouteStopsRequest{RouteId: "  "})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, 0, repo.calls)
}
