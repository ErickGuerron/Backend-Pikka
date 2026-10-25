//go:build integration

package persistence_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// placeInZone registers a PENDING order of the zone and returns its stored state.
func placeInZone(t *testing.T, store *persistence.Store, customer string, zone string, p domain.Product) domain.Order {
	t.Helper()
	order := newOrder(t, customer)
	order.ZoneID = zone
	placed, err := store.PlaceOrder(context.Background(), order, []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	return placed
}

// The waiting line is FIFO and a freed route slot takes the oldest waiting order of its zone.
func TestZoneQueueRoutesOldestWaitingIntoFreedSlots(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(100))
	orders := persistence.NewOrderRepository(pool)
	zone := uuid.NewString()
	const route = "zone#1"
	const maxPerRoute = 2

	// One order already holds a slot of the route, so the route has room for one more.
	holder := placeInZone(t, store, customer, zone, p)
	_, err := orders.AssignToRoute(ctx, []string{holder.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(route, 1)
	})
	require.NoError(t, err)

	// Five waiting orders, registered in this order.
	var waiting []domain.Order
	for range 5 {
		waiting = append(waiting, placeInZone(t, store, customer, zone, p))
	}

	routed, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", maxPerRoute)
	require.NoError(t, err)
	require.Equal(t, 1, routed, "one slot was free, so exactly the oldest waiting order is routed")

	first := getOrder(t, orders, waiting[0].ID, customer)
	require.Equal(t, domain.StatusRouteAssigned, first.Status)
	require.Equal(t, route, first.RouteID)
	require.Equal(t, 2, first.StopSequence, "the next stop after the holder")

	// The rest keep their FIFO place.
	pos, err := orders.QueuePosition(ctx, getOrder(t, orders, waiting[1].ID, customer))
	require.NoError(t, err)
	require.Equal(t, 1, pos)
	pos, err = orders.QueuePosition(ctx, getOrder(t, orders, waiting[4].ID, customer))
	require.NoError(t, err)
	require.Equal(t, 4, pos)

	// Delivering the holder frees its slot: the next waiting order takes it, with the next sequence.
	closeStop(t, orders, holder.ID, route, customer)
	routed, err = orders.FillZoneQueue(ctx, zone, "2030-04-12", maxPerRoute)
	require.NoError(t, err)
	require.Equal(t, 1, routed)
	second := getOrder(t, orders, waiting[1].ID, customer)
	require.Equal(t, domain.StatusRouteAssigned, second.Status)
	require.Equal(t, 3, second.StopSequence)

	// A full route takes nothing more.
	routed, err = orders.FillZoneQueue(ctx, zone, "2030-04-12", maxPerRoute)
	require.NoError(t, err)
	require.Equal(t, 0, routed)
}

// Zero capacity means the zone offers no route slot: nothing is routed and nothing changes.
func TestZoneQueueZeroCapacityRoutesNothing(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Pera", stock(10))
	orders := persistence.NewOrderRepository(pool)
	zone := uuid.NewString()
	waiting := placeInZone(t, store, customer, zone, p)

	routed, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", 0)
	require.NoError(t, err)
	require.Equal(t, 0, routed)
	require.Equal(t, domain.StatusPending, getOrder(t, orders, waiting.ID, customer).Status)
}

func getOrder(t *testing.T, orders *persistence.OrderRepository, id, customer string) domain.Order {
	t.Helper()
	got, err := orders.GetForCustomer(context.Background(), id, uuid.MustParse(customer))
	require.NoError(t, err)
	return got
}

// closeStop walks the route stop to DELIVERED the way the driver does: picked up, in transit, delivered.
func closeStop(t *testing.T, orders *persistence.OrderRepository, id, route, customer string) {
	t.Helper()
	ctx := context.Background()
	steps := []func(domain.Order) (domain.Order, error){
		func(o domain.Order) (domain.Order, error) { return o.AdvanceRouteStop(domain.StatusPickedUp) },
		func(o domain.Order) (domain.Order, error) { return o.AdvanceRouteStop(domain.StatusInTransit) },
		func(o domain.Order) (domain.Order, error) { return o.RecordStopOutcome(domain.StatusDelivered) },
	}
	for _, step := range steps {
		_, err := orders.AdvanceRouteStop(ctx, id, route, step)
		require.NoError(t, err)
	}
	require.Equal(t, domain.StatusDelivered, getOrder(t, orders, id, customer).Status)
}
