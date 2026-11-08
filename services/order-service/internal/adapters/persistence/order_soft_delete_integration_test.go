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

// Borrado logico: un pedido con deleted_at no vuelve en ninguna lectura, pero la fila sigue en la base.
// Nada se borra fisicamente.
func TestPedidoBorradoLogicamenteNoSeLee(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE orders SET deleted_at = now() WHERE id = $1`, placed.ID)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	_, err = orders.GetForCustomer(ctx, placed.ID, uuid.MustParse(customer))
	assert.ErrorIs(t, err, domain.ErrOrderNotFound, "GetForCustomer no debe devolver un pedido borrado")

	list, err := orders.ListByCustomer(ctx, uuid.MustParse(customer))
	require.NoError(t, err)
	assert.Empty(t, list, "ListByCustomer no debe incluir pedidos borrados")

	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM orders WHERE id = $1`, placed.ID), "la fila sigue existiendo")
}
