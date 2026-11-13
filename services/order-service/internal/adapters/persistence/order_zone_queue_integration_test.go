//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/migrations"
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

	routed, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{Daily: 10, PerRoute: maxPerRoute})
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
	routed, err = orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{Daily: 10, PerRoute: maxPerRoute})
	require.NoError(t, err)
	require.Equal(t, 1, routed)
	second := getOrder(t, orders, waiting[1].ID, customer)
	require.Equal(t, domain.StatusRouteAssigned, second.Status)
	require.Equal(t, 3, second.StopSequence)

	// A full route takes nothing more.
	routed, err = orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{Daily: 10, PerRoute: maxPerRoute})
	require.NoError(t, err)
	require.Equal(t, 0, routed)
}

// The capacity is the zone's total of active orders per day, not per route: two routes that together
// hold the zone's capacity leave no room for a third route to take a waiting order.
func TestZoneQueueCapacityIsZoneTotalAcrossRoutes(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Mango", stock(100))
	orders := persistence.NewOrderRepository(pool)
	zone := uuid.NewString()
	const maxZone = 2

	holderA := placeInZone(t, store, customer, zone, p)
	_, err := orders.AssignToRoute(ctx, []string{holderA.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute("zone#A", 1)
	})
	require.NoError(t, err)
	holderB := placeInZone(t, store, customer, zone, p)
	_, err = orders.AssignToRoute(ctx, []string{holderB.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute("zone#B", 1)
	})
	require.NoError(t, err)

	waiting := placeInZone(t, store, customer, zone, p)
	routed, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{Daily: maxZone, PerRoute: 10})
	require.NoError(t, err)
	require.Equal(t, 0, routed, "the zone already holds maxZone active orders across its routes")
	require.Equal(t, domain.StatusPending, getOrder(t, orders, waiting.ID, customer).Status)

	// Delivering one holder frees one zone slot; the waiting order takes it on the route with fewer active orders.
	closeStop(t, orders, holderA.ID, "zone#A", customer)
	routed, err = orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{Daily: maxZone, PerRoute: 10})
	require.NoError(t, err)
	require.Equal(t, 1, routed)
	require.Equal(t, "zone#A", getOrder(t, orders, waiting.ID, customer).RouteID, "zone#A has no active orders left")
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

	routed, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{})
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

type queueLimits domain.ZoneLimits

func (limits queueLimits) ZoneLimits(context.Context, string) (domain.ZoneLimits, error) {
	return domain.ZoneLimits(limits), nil
}

func TestWaitingZoneDaysAndQueueEdit(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	other := newCustomer(t, pool)
	orders := persistence.NewOrderRepository(pool)
	zoneA, zoneB := uuid.NewString(), uuid.NewString()
	product := newProduct(t, store, "Papaya", stock(10))
	first := placeInZone(t, store, customer, zoneA, product)
	second := placeInZone(t, store, customer, zoneB, product)
	third := placeInZone(t, store, customer, zoneB, product)

	days, err := orders.WaitingZoneDays(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.ZoneDay{{ZoneID: zoneA, DeliveryDate: "2030-04-12"}, {ZoneID: zoneB, DeliveryDate: "2030-04-12"}}, days)

	_, err = orders.EditForCustomer(ctx, first.ID, uuid.MustParse(other), func(o domain.Order) (domain.Order, error) {
		t.Fatal("must not read someone else's order")
		return o, nil
	})
	require.ErrorIs(t, err, domain.ErrOrderNotFound)

	updated, err := orders.EditForCustomer(ctx, first.ID, uuid.MustParse(customer), func(o domain.Order) (domain.Order, error) {
		o.ZoneID = zoneB
		return o, nil
	})
	require.NoError(t, err)
	require.Equal(t, first.CreatedAt, updated.CreatedAt)
	place, err := orders.QueuePosition(ctx, updated)
	require.NoError(t, err)
	require.Equal(t, 3, place)

	updated, err = orders.EditForCustomer(ctx, second.ID, uuid.MustParse(customer), func(o domain.Order) (domain.Order, error) {
		o.PostalCode = "170150"
		return o, nil
	})
	require.NoError(t, err)
	require.Equal(t, "170150", updated.PostalCode)
	place, err = orders.QueuePosition(ctx, updated)
	require.NoError(t, err)
	require.Equal(t, 1, place)

	updated, err = orders.EditForCustomer(ctx, third.ID, uuid.MustParse(customer), func(o domain.Order) (domain.Order, error) {
		o.DeliveryDate, err = domain.ParseDeliveryDate("2030-04-13")
		return o, err
	})
	require.NoError(t, err)
	require.Equal(t, "2030-04-13", updated.DeliveryDate.String())
	days, err = orders.WaitingZoneDays(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.ZoneDay{{ZoneID: zoneB, DeliveryDate: "2030-04-12"}, {ZoneID: zoneB, DeliveryDate: "2030-04-13"}}, days)
}

func TestPlanningAndQueueShareDailyAndRouteLimits(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	orders := persistence.NewOrderRepository(pool)
	zone := uuid.NewString()
	product := newProduct(t, store, "Piña", stock(10))
	first := placeInZone(t, store, customer, zone, product)
	second := placeInZone(t, store, customer, zone, product)
	third := placeInZone(t, store, customer, zone, product)
	limits := domain.ZoneLimits{Daily: 2, PerRoute: 1}
	assign := func(ids []string, route string) error {
		_, err := orders.AssignWithinCapacity(ctx, ids, route, queueLimits(limits), func(o domain.Order) (domain.Order, error) {
			return o.AssignToRoute(route, 1)
		})
		return err
	}
	require.ErrorIs(t, assign([]string{third.ID}, "r1"), domain.ErrZoneCapacityChanged)
	require.ErrorIs(t, assign([]string{first.ID, second.ID}, "r1"), domain.ErrRouteCapacityChanged)
	require.NoError(t, assign([]string{first.ID}, "r1"))

	assigned, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", limits)
	require.NoError(t, err)
	require.Zero(t, assigned, "the zone has room but its only route is full")
	require.NoError(t, assign([]string{second.ID}, "r2"))
	require.ErrorIs(t, assign([]string{third.ID}, "r3"), domain.ErrZoneCapacityChanged)
	active, err := orders.ZoneActiveCount(ctx, zone, "2030-04-12")
	require.NoError(t, err)
	require.Equal(t, 2, active)
	require.Equal(t, domain.StatusPending, getOrder(t, orders, third.ID, customer).Status)

	closeStop(t, orders, first.ID, "r1", customer)
	results := make(chan error, 2)
	go func() {
		_, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", limits)
		results <- err
	}()
	go func() { results <- assign([]string{third.ID}, "r3") }()
	for range 2 {
		err := <-results
		if !errors.Is(err, domain.ErrOrderNotPending) {
			require.NoError(t, err)
		}
	}
	active, err = orders.ZoneActiveCount(ctx, zone, "2030-04-12")
	require.NoError(t, err)
	require.Equal(t, 2, active)
	require.Equal(t, domain.StatusRouteAssigned, getOrder(t, orders, third.ID, customer).Status)
}

func TestZoneQueueSkipsARouteWithoutAReservedDriver(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	zone := uuid.NewString()
	orders := persistence.NewOrderRepository(pool).WithRouteReservation(func(_ context.Context, route string) (bool, error) {
		return route == "z-reserved", nil
	})
	product := newProduct(t, store, "Uva", stock(10))
	for _, route := range []string{"a-unreserved", "z-reserved"} {
		holder := placeInZone(t, store, customer, zone, product)
		_, err := orders.AssignToRoute(ctx, []string{holder.ID}, func(o domain.Order) (domain.Order, error) {
			return o.AssignToRoute(route, 1)
		})
		require.NoError(t, err)
		closeStop(t, orders, holder.ID, route, customer)
	}
	waiting := placeInZone(t, store, customer, zone, product)
	assigned, err := orders.FillZoneQueue(ctx, zone, "2030-04-12", domain.ZoneLimits{Daily: 3, PerRoute: 1})
	require.NoError(t, err)
	require.Equal(t, 1, assigned)
	require.Equal(t, "z-reserved", getOrder(t, orders, waiting.ID, customer).RouteID)
}

func TestZoneQueueMigrationBackfillsOriginalRegistrationTimes(t *testing.T) {
	_, pool := setup(t)
	ctx := context.Background()
	apply := func(name string) {
		t.Helper()
		sql, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(sql))
		require.NoError(t, err)
	}

	apply("000028_order_queued_at.down.sql")
	var id string
	var createdAt time.Time
	err := pool.QueryRow(ctx, `INSERT INTO orders
		(address, latitude, longitude, delivery_date, window_start, window_end, created_at)
		VALUES ('Av. Central 123', -0.2, -78.5, '2030-04-12', '09:00', '12:00', '2025-10-15T12:00:00Z')
		RETURNING id::text, created_at`).Scan(&id, &createdAt)
	require.NoError(t, err)

	apply("000028_order_queued_at.up.sql")
	var storedCreation, queueEntry time.Time
	err = pool.QueryRow(ctx, `SELECT created_at, queued_at FROM orders WHERE id = $1`, id).Scan(&storedCreation, &queueEntry)
	require.NoError(t, err)
	require.Equal(t, createdAt, storedCreation)
	require.Equal(t, createdAt, queueEntry)
}
