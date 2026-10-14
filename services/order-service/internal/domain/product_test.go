package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var now = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

func price(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func stock(n int64) *int64 { return &n }

func newApple(t *testing.T, initial *int64) (domain.Product, *domain.StockMovement) {
	t.Helper()
	p, movement, err := domain.NewProduct(uuid.New(), domain.NewProductInput{
		Name: "Manzana", UnitPrice: price("0.50"), InitialStock: initial,
	}, nil, now)
	require.NoError(t, err)
	return p, movement
}

func TestNewProductConStockInicialRegistraEntrada(t *testing.T) {
	p, movement := newApple(t, stock(4))

	require.NotNil(t, p.StockQuantity)
	assert.Equal(t, int64(4), *p.StockQuantity)
	require.NotNil(t, movement)
	assert.Equal(t, domain.MovementRestock, movement.Type)
	assert.Equal(t, int64(4), movement.Delta)
	assert.Equal(t, int64(4), movement.ResultingStock)
}

func TestNewProductSinStockNoControlaExistencias(t *testing.T) {
	p, movement := newApple(t, nil)

	assert.Nil(t, p.StockQuantity)
	assert.Nil(t, movement)
	assert.True(t, p.Active)
}

func TestNewProductValidaNombrePrecioYStock(t *testing.T) {
	id := uuid.New()
	tests := []struct {
		name  string
		input domain.NewProductInput
		want  error
	}{
		{name: "nombre vacío", input: domain.NewProductInput{Name: "  ", UnitPrice: price("1")}, want: domain.ErrInvalidProductName},
		{name: "nombre demasiado largo", input: domain.NewProductInput{Name: strings.Repeat("a", 121), UnitPrice: price("1")}, want: domain.ErrInvalidProductName},
		{name: "precio cero", input: domain.NewProductInput{Name: "x", UnitPrice: price("0")}, want: domain.ErrInvalidPrice},
		{name: "precio negativo", input: domain.NewProductInput{Name: "x", UnitPrice: price("-1")}, want: domain.ErrInvalidPrice},
		{name: "precio con tres decimales", input: domain.NewProductInput{Name: "x", UnitPrice: price("1.005")}, want: domain.ErrInvalidPrice},
		{name: "stock inicial negativo", input: domain.NewProductInput{Name: "x", UnitPrice: price("1"), InitialStock: stock(-1)}, want: domain.ErrInvalidStock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := domain.NewProduct(id, tt.input, nil, now)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

// El ejemplo del negocio: 4 manzanas, se entregan 2 (salida) y entra 1 (entrada) = 3.
func TestHistorialDeManzanas(t *testing.T) {
	p, _ := newApple(t, stock(4))

	sale, err := p.ApplyMovement(domain.MovementSale, -2, "", nil, nil, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), sale.ResultingStock)

	restock, err := p.ApplyMovement(domain.MovementRestock, 1, "compra del martes", nil, nil, now)
	require.NoError(t, err)
	assert.Equal(t, int64(3), restock.ResultingStock)
	assert.Equal(t, int64(3), *p.StockQuantity)
}

func TestAjusteManualPideMotivoYPuedeRestar(t *testing.T) {
	p, _ := newApple(t, stock(3))

	_, err := p.ApplyMovement(domain.MovementAdjustment, -1, "  ", nil, nil, now)
	assert.ErrorIs(t, err, domain.ErrAdjustmentReasonRequired)

	adjust, err := p.ApplyMovement(domain.MovementAdjustment, -1, "producto dañado", nil, nil, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), adjust.ResultingStock)
	assert.Equal(t, "producto dañado", adjust.Reason)
}

func TestNoSeVendeMasDeLoQueHay(t *testing.T) {
	p, _ := newApple(t, stock(1))

	_, err := p.ApplyMovement(domain.MovementSale, -2, "", nil, nil, now)

	assert.ErrorIs(t, err, domain.ErrInsufficientStock)
	assert.Equal(t, int64(1), *p.StockQuantity, "un rechazo no cambia el stock")
}

func TestAjusteNoPuedeDejarStockNegativo(t *testing.T) {
	p, _ := newApple(t, stock(1))

	_, err := p.ApplyMovement(domain.MovementAdjustment, -5, "conteo", nil, nil, now)

	assert.ErrorIs(t, err, domain.ErrInsufficientStock)
}

func TestProductoSinControlNoAceptaMovimientos(t *testing.T) {
	p, _ := newApple(t, nil)

	_, err := p.ApplyMovement(domain.MovementRestock, 1, "", nil, nil, now)

	assert.ErrorIs(t, err, domain.ErrStockNotTracked)
}

func TestTipoYSignoDebenCoincidir(t *testing.T) {
	p, _ := newApple(t, stock(5))
	tests := []struct {
		name  string
		kind  domain.MovementType
		delta int64
	}{
		{name: "entrada negativa", kind: domain.MovementRestock, delta: -1},
		{name: "venta positiva", kind: domain.MovementSale, delta: 1},
		{name: "devolución negativa", kind: domain.MovementReturn, delta: -1},
		{name: "ajuste en cero", kind: domain.MovementAdjustment, delta: 0},
		{name: "tipo desconocido", kind: domain.MovementType("TRANSFER"), delta: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.ApplyMovement(tt.kind, tt.delta, "motivo", nil, nil, now)
			assert.ErrorIs(t, err, domain.ErrInvalidMovement)
		})
	}
}

func TestDevolucionSumaAlStock(t *testing.T) {
	p, _ := newApple(t, stock(2))
	orderID := uuid.New()

	ret, err := p.ApplyMovement(domain.MovementReturn, 2, "pedido cancelado", &orderID, nil, now)

	require.NoError(t, err)
	assert.Equal(t, int64(4), ret.ResultingStock)
	require.NotNil(t, ret.OrderID)
	assert.Equal(t, orderID, *ret.OrderID)
}

func TestActivarYDesactivarProducto(t *testing.T) {
	p, _ := newApple(t, nil)

	p.Deactivate(now)
	assert.False(t, p.Active)
	p.Activate(now)
	assert.True(t, p.Active)
}
