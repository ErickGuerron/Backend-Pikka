package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func activeProduct(t *testing.T, unit string) domain.Product {
	t.Helper()
	p, _, err := domain.NewProduct(uuid.New(), domain.NewProductInput{Name: "Pan", UnitPrice: price(unit)}, nil, now)
	require.NoError(t, err)
	return p
}

func TestLineaCopiaPrecioYNombreDelMomento(t *testing.T) {
	p := activeProduct(t, "1.25")

	line, err := domain.NewOrderLine(p, 3)

	require.NoError(t, err)
	assert.Equal(t, p.ID, line.ProductID)
	assert.Equal(t, "Pan", line.Name)
	assert.True(t, line.UnitPrice.Equal(decimal.RequireFromString("1.25")))
	assert.True(t, line.Subtotal().Equal(decimal.RequireFromString("3.75")))
}

func TestLineaValidaCantidadYProductoActivo(t *testing.T) {
	p := activeProduct(t, "1")

	_, err := domain.NewOrderLine(p, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidQuantity)
	_, err = domain.NewOrderLine(p, domain.MaxLineQuantity+1)
	assert.ErrorIs(t, err, domain.ErrInvalidQuantity)

	p.Deactivate(now)
	_, err = domain.NewOrderLine(p, 1)
	assert.ErrorIs(t, err, domain.ErrProductInactive)
}

func TestTotalDelPedido(t *testing.T) {
	a, _ := domain.NewOrderLine(activeProduct(t, "0.50"), 4)
	b, _ := domain.NewOrderLine(activeProduct(t, "2.00"), 1)

	total, err := domain.OrderTotal([]domain.OrderLine{a, b})

	require.NoError(t, err)
	assert.True(t, total.Equal(decimal.RequireFromString("4.00")))
}

func TestPedidoSinProductosNoTieneTotal(t *testing.T) {
	_, err := domain.OrderTotal(nil)
	assert.ErrorIs(t, err, domain.ErrNoItems)
}
