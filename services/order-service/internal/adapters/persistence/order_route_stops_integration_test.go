//go:build integration

package persistence_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// CountStopsBefore cuenta solo las paradas abiertas de la misma ruta con secuencia menor que la del pedido.
func TestCountStopsBeforeCuentaParadasAbiertasAntesDeLaMia(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(20))
	orders := persistence.NewOrderRepository(pool)

	// Ruta testRoute: 1 entregado, 2 no entregado, 3 cancelado, 4 recogido, 5 en tránsito, 6 mío, 7 después.
	seq := map[int]string{}
	statuses := map[int]string{1: "DELIVERED", 2: "NOT_DELIVERED", 3: "CANCELLED", 4: "PICKED_UP", 5: "IN_TRANSIT", 6: "ROUTE_ASSIGNED", 7: "ROUTE_ASSIGNED"}
	for n := 1; n <= 7; n++ {
		placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
		require.NoError(t, err)
		seq[n] = placed.ID
		_, err = orders.AssignToRoute(ctx, []string{placed.ID}, func(o domain.Order) (domain.Order, error) {
			return o.AssignToRoute(testRoute, n)
		})
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE orders SET status = $2 WHERE id = $1`, uuid.MustParse(placed.ID), statuses[n])
		require.NoError(t, err)
	}

	// Un pedido abierto con secuencia menor en otra ruta no cuenta.
	other, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	_, err = orders.AssignToRoute(ctx, []string{other.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute("zone-2#1", 1)
	})
	require.NoError(t, err)

	// Un pedido sin ruta no tiene paradas delante.
	unrouted, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)

	got, err := orders.CountStopsBefore(ctx, seq[6], uuid.MustParse(customer))
	require.NoError(t, err)
	assert.Equal(t, 2, got, "solo cuentan 4 (recogido) y 5 (en tránsito)")

	got, err = orders.CountStopsBefore(ctx, unrouted.ID, uuid.MustParse(customer))
	require.NoError(t, err)
	assert.Equal(t, 0, got, "un pedido sin ruta no tiene paradas delante")
}

// Un pedido ajeno responde como inexistente, sin revelar su conteo.
func TestCountStopsBeforeDeOtroClienteNoExiste(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	owner := newCustomer(t, pool)
	stranger := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	placed, err := store.PlaceOrder(ctx, newOrder(t, owner), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)
	assignOne(t, orders, placed.ID)

	_, err = orders.CountStopsBefore(ctx, placed.ID, uuid.MustParse(stranger))

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestReorderDriverRoutePreservesClosedSlotsAndUpdatesCustomerPosition(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	product := newProduct(t, store, "Manzana", stock(10))
	orders := persistence.NewOrderRepository(pool)
	zoneID := uuid.NewString()
	var stops []domain.Order
	for i := 1; i <= 3; i++ {
		draft := newOrder(t, customer)
		draft.ZoneID = zoneID
		placed, err := store.PlaceOrder(ctx, draft, []domain.OrderLine{lineFor(t, product, 1)}, nil, nil)
		require.NoError(t, err)
		assigned, err := orders.AssignToRoute(ctx, []string{placed.ID}, func(o domain.Order) (domain.Order, error) {
			return o.AssignToRoute(testRoute, i)
		})
		require.NoError(t, err)
		stops = append(stops, assigned[0])
	}
	_, err := pool.Exec(ctx, `UPDATE orders SET status = 'DELIVERED' WHERE id = $1`, stops[1].ID)
	require.NoError(t, err)

	proposed := []uuid.UUID{uuid.MustParse(stops[2].ID), uuid.MustParse(stops[0].ID)}
	versions := map[string]int64{stops[0].ID: stops[0].Version, stops[2].ID: stops[2].Version}
	actor := uuid.NewString()
	ok, err := orders.ReorderDriverRoute(ctx, testRoute, actor, "Cliente disponible ahora", proposed, versions)
	require.NoError(t, err)
	require.True(t, ok)
	got, err := orders.ListDriverRouteStops(ctx, testRoute)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{stops[2].ID, stops[1].ID, stops[0].ID}, []string{got[0].ID, got[1].ID, got[2].ID})
	assert.Equal(t, 2, got[1].StopSequence)
	ahead, err := orders.CountStopsBefore(ctx, stops[2].ID, uuid.MustParse(customer))
	require.NoError(t, err)
	assert.Zero(t, ahead)
	ok, err = orders.ReorderDriverRoute(ctx, testRoute, actor, "Versión anterior", proposed, versions)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM driver_route_reorders WHERE actor_id = $1`, actor))
}
