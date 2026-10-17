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

const testRoute = "zone-1#1"

func assignOne(t *testing.T, orders *persistence.OrderRepository, id string) {
	t.Helper()
	_, err := orders.AssignToRoute(context.Background(), []string{id}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 0)
	})
	require.NoError(t, err)
}

// GetPendingByDate devuelve solo los PENDING de la fecha pedida.
func TestPendientesSoloPendingDeLaFecha(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	pending, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	assigned, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)
	assignOne(t, orders, assigned.ID)

	got, err := orders.ListPendingByDate(ctx, "2030-04-12")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, pending.ID, got[0].ID)
	assert.Equal(t, domain.StatusPending, got[0].Status)

	other, err := orders.ListPendingByDate(ctx, "2030-04-13")
	require.NoError(t, err)
	assert.Empty(t, other)
}

// Todo o nada: un pedido que no está en PENDING deja a los demás sin cambios.
func TestAsignarRutaTodoONinguno(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	free, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	taken, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)
	assignOne(t, orders, taken.ID)

	_, err = orders.AssignToRoute(ctx, []string{free.ID, taken.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 0)
	})

	assert.ErrorIs(t, err, domain.ErrOrderNotPending)
	left, err := orders.ListPendingByDate(ctx, "2030-04-12")
	require.NoError(t, err)
	require.Len(t, left, 1, "el pedido libre sigue PENDING")
	assert.Equal(t, free.ID, left[0].ID)
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM orders WHERE id = $1 AND route_id IS NOT NULL`, free.ID), "sin ruta asignada")
}

// Un id desconocido responde NOT_FOUND y no cambia ningún pedido.
func TestAsignarRutaIDDesconocido(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	free, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	_, err = orders.AssignToRoute(ctx, []string{free.ID, uuid.NewString()}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 0)
	})

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
	left, err := orders.ListPendingByDate(ctx, "2030-04-12")
	require.NoError(t, err)
	assert.Len(t, left, 1)
}

// Éxito: los pedidos quedan ROUTE_ASSIGNED con su ruta, en el orden recibido.
func TestAsignarRutaOK(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	first, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	second, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	got, err := orders.AssignToRoute(ctx, []string{second.ID, first.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 0)
	})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, second.ID, got[0].ID)
	assert.Equal(t, first.ID, got[1].ID)
	for _, o := range got {
		assert.Equal(t, domain.StatusRouteAssigned, o.Status)
		assert.Equal(t, testRoute, o.RouteID)
	}
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM orders WHERE route_id = $1 AND status = 'ROUTE_ASSIGNED'`, testRoute), "la ruta queda guardada en la base")
	left, err := orders.ListPendingByDate(ctx, "2030-04-12")
	require.NoError(t, err)
	assert.Empty(t, left)
}
