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

// El cliente lee su propio pedido, con el route_id que guardó Routing.
func TestLeerPedidoPropioIncluyeRouteID(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE orders SET route_id = 'route-s1' WHERE id = $1`, placed.ID)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	got, err := orders.GetForCustomer(ctx, placed.ID, uuid.MustParse(customer))

	require.NoError(t, err)
	assert.Equal(t, placed.ID, got.ID)
	assert.Equal(t, "route-s1", got.RouteID)
}

// Un pedido ajeno no se devuelve y responde como inexistente (BOLA, ADR 0007).
func TestLeerPedidoAjenoNoExiste(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	owner := newCustomer(t, pool)
	stranger := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(5))
	placed, err := store.PlaceOrder(ctx, newOrder(t, owner), []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)
	require.NoError(t, err)
	orders := persistence.NewOrderRepository(pool)

	_, err = orders.GetForCustomer(ctx, placed.ID, uuid.MustParse(stranger))

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
}

// Un id que no es UUID responde como inexistente, sin consultar la base.
func TestLeerPedidoIDInvalidoNoExiste(t *testing.T) {
	_, pool := setup(t)
	orders := persistence.NewOrderRepository(pool)

	_, err := orders.GetForCustomer(context.Background(), "no-es-uuid", uuid.New())

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
}
