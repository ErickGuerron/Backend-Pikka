//go:build integration

package persistence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// placeInReview guarda un pedido en PENDING_REVIEW con productos, como lo hace el alta cuando la zona no cubre.
func placeInReview(t *testing.T, store *persistence.Store, customer string, p domain.Product, quantity int64, reasonCode string) domain.Order {
	t.Helper()
	reason, err := domain.ParseReviewReason(reasonCode)
	require.NoError(t, err)
	order := newOrder(t, customer)
	order, err = order.ApplyValidation([]domain.ReviewReason{reason})
	require.NoError(t, err)
	placed, err := store.PlaceOrder(context.Background(), order, []domain.OrderLine{lineFor(t, p, quantity)}, nil, nil)
	require.NoError(t, err)
	return placed
}

// El rechazo del operador cancela el pedido y devuelve el stock, en la misma transacción.
func TestRechazarPedidoDevuelveStock(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed := placeInReview(t, store, customer, p, 2, "ZONE_NOT_COVERED")
	orders := persistence.NewOrderRepository(pool)

	got, err := orders.Review(ctx, placed.ID, func(o domain.Order) (domain.Order, *domain.Notification, error) {
		next, err := o.Reject()
		return next, nil, err
	})

	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, got.Status)
	product, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), *product.StockQuantity, "el stock vuelve a 5")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE order_id = $1 AND movement_type = 'RETURN' AND delta = 2`, placed.ID))
}

// Confirmar un pedido no devuelve stock: el producto sí se entrega.
func TestConfirmarPedidoNoDevuelveStock(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed := placeInReview(t, store, customer, p, 2, "INVALID_WINDOW")
	orders := persistence.NewOrderRepository(pool)

	_, err := orders.Review(ctx, placed.ID, func(o domain.Order) (domain.Order, *domain.Notification, error) {
		next, err := o.Confirm()
		return next, nil, err
	})

	require.NoError(t, err)
	product, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), *product.StockQuantity)
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE order_id = $1 AND movement_type = 'RETURN'`, placed.ID))
}
