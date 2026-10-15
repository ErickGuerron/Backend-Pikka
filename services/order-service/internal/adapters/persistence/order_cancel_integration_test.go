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

// Cancelar el propio pedido en PENDING devuelve el stock en la misma transacción.
func TestCancelarPedidoPropioDevuelveStock(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	order := newOrder(t, customer)
	placed, err := store.PlaceOrder(ctx, order, []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	got, err := orders.CancelForCustomer(ctx, placed.ID, uuid.MustParse(customer), func(o domain.Order) (domain.Order, error) {
		return o.CancelByCustomer()
	})

	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, got.Status)
	product, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), *product.StockQuantity, "el stock vuelve a 5")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE order_id = $1 AND movement_type = 'RETURN' AND delta = 2`, placed.ID))
}

// Un pedido ajeno no se toca y responde como inexistente (BOLA, ADR 0007).
func TestCancelarPedidoAjenoNoExiste(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	owner := newCustomer(t, pool)
	stranger := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed, err := store.PlaceOrder(ctx, newOrder(t, owner), []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	_, err = orders.CancelForCustomer(ctx, placed.ID, uuid.MustParse(stranger), func(o domain.Order) (domain.Order, error) {
		return o.CancelByCustomer()
	})

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
	product, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), *product.StockQuantity, "el stock no cambia")
}

// Un pedido ya despachado no se cancela y el stock no se devuelve.
func TestCancelarPedidoDespachadoFallaSinDevolverStock(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE orders SET status = 'PICKED_UP' WHERE id = $1`, placed.ID)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	_, err = orders.CancelForCustomer(ctx, placed.ID, uuid.MustParse(customer), func(o domain.Order) (domain.Order, error) {
		return o.CancelByCustomer()
	})

	assert.ErrorIs(t, err, domain.ErrOrderNotCancellable)
	product, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), *product.StockQuantity)
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE order_id = $1 AND movement_type = 'RETURN'`, placed.ID))
}
