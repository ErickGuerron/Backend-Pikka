package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// fakeRouteCapacity answers the route capacity of a zone and records which zone was asked.
type fakeRouteCapacity struct {
	max    int
	err    error
	zoneID string
}

func (f *fakeRouteCapacity) MaxOrdersPerRoute(_ context.Context, zoneID string) (int, error) {
	f.zoneID = zoneID
	return f.max, f.err
}

func routeStopsInZone(zoneID string) []domain.Order {
	return []domain.Order{
		{ID: "o-1", Address: "Av. Amazonas 100", StopSequence: 1, Status: domain.StatusRouteAssigned, ZoneID: zoneID},
		{ID: "o-2", Address: "Calle Larga 5", StopSequence: 2, Status: domain.StatusRouteAssigned, ZoneID: zoneID},
	}
}

func TestListDriverRouteStopsReportsTheZoneRouteCapacity(t *testing.T) {
	repo := &driverRouteServerRepo{stops: routeStopsInZone("zone-9")}
	capacity := &fakeRouteCapacity{max: 10}
	server := NewOrderServer(OrderUseCases{DriverRouteStops: orderapp.NewDriverRouteStops(repo)}).WithRouteCapacity(capacity)

	res, err := server.ListDriverRouteStops(identityCtx(uuid.New(), "DRIVER"), &ordersv1.ListDriverRouteStopsRequest{RouteId: "route-7"})

	require.NoError(t, err)
	assert.Equal(t, int32(10), res.GetMaxOrders())
	assert.Equal(t, "zone-9", capacity.zoneID, "the capacity is the one of the route's zone")
}

func TestListDriverRouteStopsKeepsStopsWhenCapacityIsUnavailable(t *testing.T) {
	// Zone Service caido no debe dejar al repartidor sin ruta: solo se oculta el maximo (0 = desconocido).
	repo := &driverRouteServerRepo{stops: routeStopsInZone("zone-9")}
	capacity := &fakeRouteCapacity{err: errors.New("zone service down")}
	server := NewOrderServer(OrderUseCases{DriverRouteStops: orderapp.NewDriverRouteStops(repo)}).WithRouteCapacity(capacity)

	res, err := server.ListDriverRouteStops(identityCtx(uuid.New(), "DRIVER"), &ordersv1.ListDriverRouteStopsRequest{RouteId: "route-7"})

	require.NoError(t, err)
	assert.Len(t, res.GetStops(), 2)
	assert.Equal(t, int32(0), res.GetMaxOrders())
}

func TestListDriverRouteStopsWithoutCapacityWiredReportsZero(t *testing.T) {
	repo := &driverRouteServerRepo{stops: routeStopsInZone("zone-9")}
	server := newDriverRouteServer(repo)

	res, err := server.ListDriverRouteStops(identityCtx(uuid.New(), "DRIVER"), &ordersv1.ListDriverRouteStopsRequest{RouteId: "route-7"})

	require.NoError(t, err)
	assert.Equal(t, int32(0), res.GetMaxOrders())
}
