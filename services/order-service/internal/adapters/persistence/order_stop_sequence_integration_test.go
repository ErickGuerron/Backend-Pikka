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

// La secuencia de parada se guarda con el paso a ROUTE_ASSIGNED y vuelve en las lecturas.
func TestAsignarRutaGuardaSecuenciaDeParada(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	first, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	second, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	got, err := orders.AssignToRoute(ctx, []string{first.ID, second.ID}, func(o domain.Order) (domain.Order, error) {
		if o.ID == first.ID {
			return o.AssignToRoute(testRoute, 1)
		}
		return o.AssignToRoute(testRoute, 2)
	})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 1, got[0].StopSequence)
	assert.Equal(t, 2, got[1].StopSequence)
	read, err := orders.GetForCustomer(ctx, second.ID, uuid.MustParse(customer))
	require.NoError(t, err)
	assert.Equal(t, 2, read.StopSequence, "la lectura devuelve la secuencia guardada")
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM orders WHERE route_id = $1 AND stop_sequence IS NOT NULL`, testRoute))
}

// Dos pedidos de la misma ruta no pueden ocupar la misma parada: la base lo impide y no cambia nada.
func TestAsignarRutaSecuenciaDuplicadaNoSeGuarda(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	taken, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	free, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)
	_, err = orders.AssignToRoute(ctx, []string{taken.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 1)
	})
	require.NoError(t, err)

	_, err = orders.AssignToRoute(ctx, []string{free.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 1)
	})

	assert.ErrorIs(t, err, domain.ErrDuplicateStopSequence)
	left, err := orders.ListPendingByDate(ctx, "2030-04-12")
	require.NoError(t, err)
	require.Len(t, left, 1, "el pedido libre sigue PENDING")
	assert.Equal(t, free.ID, left[0].ID)
}

// Un pedido en otra ruta no se puede avanzar: responde como inexistente.
func TestAvanzarParadaOtraRutaNoExiste(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)
	assignOne(t, orders, placed.ID)

	_, err = orders.AdvanceRouteStop(ctx, placed.ID, "zone-9#4", func(o domain.Order) (domain.Order, error) {
		return o.AdvanceRouteStop(domain.StatusPickedUp)
	})

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
}

// ROUTE_ASSIGNED -> PICKED_UP -> IN_TRANSIT se guarda y conserva la ruta y la secuencia.
func TestAvanzarParadaGuardaEstados(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)
	_, err = orders.AssignToRoute(ctx, []string{placed.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, 1)
	})
	require.NoError(t, err)

	for _, target := range []domain.Status{domain.StatusPickedUp, domain.StatusInTransit} {
		got, err := orders.AdvanceRouteStop(ctx, placed.ID, testRoute, func(o domain.Order) (domain.Order, error) {
			return o.AdvanceRouteStop(target)
		})
		require.NoError(t, err)
		assert.Equal(t, target, got.Status)
		assert.Equal(t, testRoute, got.RouteID)
		assert.Equal(t, 1, got.StopSequence)
	}
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM orders WHERE id = $1 AND status = 'IN_TRANSIT' AND stop_sequence = 1`, placed.ID))
}
