//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/infrastructure/postgres"
)

var now = time.Date(2030, 4, 10, 9, 0, 0, 0, time.UTC)

// setup aplica migraciones, vacía las tablas del catálogo y de pedidos, y devuelve el adaptador.
func setup(t *testing.T) (*persistence.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("ORDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORDER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	require.NoError(t, postgres.Migrate(dsn))
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `TRUNCATE customer_notifications, order_penalty_reasons, order_items, stock_movements, products, orders, customers RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	return persistence.NewStore(pool), pool
}

func stock(n int64) *int64 { return &n }

func newProduct(t *testing.T, store *persistence.Store, name string, initial *int64) domain.Product {
	t.Helper()
	p, movement, err := domain.NewProduct(uuid.New(), domain.NewProductInput{
		Name: name, UnitPrice: decimal.RequireFromString("2.50"), InitialStock: initial,
	}, nil, now)
	require.NoError(t, err)
	require.NoError(t, store.CreateProduct(context.Background(), p, movement))
	return p
}

// newCustomer crea un cliente real: el pedido lo referencia por su id.
func newCustomer(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO customers (user_id, full_name, email)
		VALUES (gen_random_uuid(), 'Ana Cliente', gen_random_uuid()::text || '@example.com')
		RETURNING id::text`).Scan(&id))
	return id
}

// newOrder arma un pedido válido de un cliente. El id lo asigna la base al guardarlo.
func newOrder(t *testing.T, customerID string) domain.Order {
	t.Helper()
	order, err := domain.NewOrder(domain.NewOrderInput{
		Address: "Avenida Amazonas 123", Latitude: -0.18, Longitude: -78.48,
		DeliveryDate: "2030-04-12", WindowStart: "09:00", WindowEnd: "12:00",
		AddressReference: "Casa azul", ContactPhone: "0991234567",
	}, now)
	require.NoError(t, err)
	order.CustomerID = customerID
	return order
}

func lineFor(t *testing.T, p domain.Product, quantity int64) domain.OrderLine {
	t.Helper()
	line, err := domain.NewOrderLine(p, quantity)
	require.NoError(t, err)
	return line
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func TestCrearProductoGuardaStockEHistorial(t *testing.T) {
	store, pool := setup(t)
	p := newProduct(t, store, "Manzana", stock(4))

	got, err := store.GetProduct(context.Background(), p.ID)

	require.NoError(t, err)
	require.NotNil(t, got.StockQuantity)
	assert.Equal(t, int64(4), *got.StockQuantity)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE product_id = $1 AND movement_type = 'RESTOCK'`, p.ID))
}

// El caso del negocio: 4 manzanas, un ajuste de −1 por daño y una entrada de +1 = 4.
func TestAjustesManualesConMotivoYSinNegativo(t *testing.T) {
	store, _ := setup(t)
	ctx := context.Background()
	p := newProduct(t, store, "Manzana", stock(4))

	_, err := store.AdjustStock(ctx, p.ID, domain.MovementAdjustment, -1, "  ", nil, now)
	assert.ErrorIs(t, err, domain.ErrAdjustmentReasonRequired)

	dañado, err := store.AdjustStock(ctx, p.ID, domain.MovementAdjustment, -1, "producto dañado", nil, now)
	require.NoError(t, err)
	assert.Equal(t, int64(3), dañado.ResultingStock)

	entrada, err := store.AdjustStock(ctx, p.ID, domain.MovementRestock, 1, "compra", nil, now)
	require.NoError(t, err)
	assert.Equal(t, int64(4), entrada.ResultingStock)

	_, err = store.AdjustStock(ctx, p.ID, domain.MovementAdjustment, -10, "conteo", nil, now)
	assert.ErrorIs(t, err, domain.ErrInsufficientStock)
}

func TestPedidoDescuentaStockYGuardaLineas(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Pan", stock(5))

	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 2)}, nil, nil)

	require.NoError(t, err)
	require.NotEmpty(t, placed.ID, "la base asigna el id del pedido")
	got, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), *got.StockQuantity)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM order_items WHERE order_id = $1`, placed.ID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE order_id = $1 AND movement_type = 'SALE' AND delta = -2`, placed.ID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM orders WHERE id = $1 AND customer_id = $2`, placed.ID, customer))
}

// Si un producto de la lista no alcanza, el pedido completo no se guarda y ningún stock cambia.
func TestPedidoEsAtomico(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	a := newProduct(t, store, "Pan", stock(5))
	b := newProduct(t, store, "Leche", stock(2))
	order := newOrder(t, customer)

	_, err := store.PlaceOrder(ctx, order, []domain.OrderLine{lineFor(t, a, 1), lineFor(t, b, 5)}, nil, nil)

	assert.ErrorIs(t, err, domain.ErrInsufficientStock)
	gotA, err := store.GetProduct(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), *gotA.StockQuantity, "el primer producto no debe quedar descontado")
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM orders WHERE customer_id = $1`, customer))
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM order_items`))
}

// Diez pedidos simultáneos sobre tres unidades: exactamente tres se aceptan y el stock queda en cero.
func TestPedidosConcurrentesNoVendenDeMas(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(3))

	const buyers = 10
	// Los pedidos y sus líneas se preparan aquí, en el hilo de la prueba: las goroutines no llaman a require.
	orders := make([]domain.Order, buyers)
	lines := make([]domain.OrderLine, buyers)
	for i := range orders {
		orders[i] = newOrder(t, customer)
		lines[i] = lineFor(t, p, 1)
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		rejected int
		others   []error
	)
	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := store.PlaceOrder(ctx, orders[n], []domain.OrderLine{lines[n]}, nil, nil)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, domain.ErrInsufficientStock):
				rejected++
			default:
				others = append(others, fmt.Errorf("pedido %d: %w", n, err))
			}
		}(i)
	}
	wg.Wait()

	assert.Empty(t, others)
	assert.Equal(t, 3, accepted)
	assert.Equal(t, buyers-3, rejected)
	got, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), *got.StockQuantity)
	assert.Equal(t, 3, countRows(t, pool, `SELECT count(*) FROM stock_movements WHERE product_id = $1 AND movement_type = 'SALE'`, p.ID))
}

// Una línea armada cuando el producto estaba activo no sirve si se desactivó antes de guardar el pedido.
func TestProductoDesactivadoNoSeVende(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Jugo", stock(5))
	line := lineFor(t, p, 1)
	require.NoError(t, store.SetActive(ctx, p.ID, false, now))

	_, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{line}, nil, nil)

	assert.ErrorIs(t, err, domain.ErrProductInactive)
	got, err := store.GetProduct(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), *got.StockQuantity)
}

// El aviso al cliente se guarda en la misma transacción que el pedido y sus productos.
func TestPedidoConAvisoGuardaNotificacion(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Pan", stock(5))
	notice := domain.NewNotification(customer, "", domain.NotificationInReview, "revisión")

	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, &notice, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM customer_notifications WHERE order_id = $1`, placed.ID))
}
