//go:build integration

package persistence

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/infrastructure/postgres"
)

// Requiere ORDER_TEST_DATABASE_URL apuntando al esquema orders. La tabla puede tener pedidos de otras
// pruebas: se comprueba solo lo que pertenece al cliente creado aquí.
func TestOrderRepositoryListAllFiltersAndOrders(t *testing.T) {
	dsn := os.Getenv("ORDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORDER_TEST_DATABASE_URL is not set")
	}
	require.NoError(t, postgres.Migrate(dsn))
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	customer, err := NewCustomerRepository(pool).Create(ctx, domain.Customer{
		UserID: uuid.New(), Email: fmt.Sprintf("listado-%s@test.local", uuid.NewString()), FullName: "Cliente de listado",
		Status: domain.StatusActive, PendingSurcharge: decimal.Zero,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE customer_id = $1`, customer.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE id = $1`, customer.ID)
	})

	repo := NewOrderRepository(pool)
	newOrder := func(date string, reasons []domain.ReviewReason) domain.Order {
		o, err := domain.NewOrder(domain.NewOrderInput{
			Address: "Av. de prueba", DeliveryDate: date, WindowStart: "09:00", WindowEnd: "12:00",
			AddressReference: "Casa azul", ContactPhone: "0991234567",
		}, time.Date(2031, 7, 1, 10, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		o.CustomerID = customer.ID.String()
		o, err = o.ApplyValidation(reasons)
		require.NoError(t, err)
		created, err := repo.Create(ctx, o, nil)
		require.NoError(t, err)
		return created
	}
	pending := newOrder("2031-07-15", nil)
	inReview := newOrder("2031-07-16", []domain.ReviewReason{domain.ReviewAddressNotLocated})
	require.Equal(t, domain.StatusPending, pending.Status)
	require.Equal(t, domain.StatusPendingReview, inReview.Status)

	mine := func(filter ports.OrderFilter) []string {
		got, err := repo.ListAll(ctx, filter)
		require.NoError(t, err)
		var ids []string
		for _, o := range got {
			if o.CustomerID == customer.ID.String() {
				ids = append(ids, o.ID)
			}
		}
		return ids
	}

	assert.Equal(t, []string{inReview.ID, pending.ID}, mine(ports.OrderFilter{}), "más reciente primero")
	assert.Equal(t, []string{pending.ID}, mine(ports.OrderFilter{DeliveryDate: "2031-07-15"}), "filtra por fecha de entrega")
	assert.Equal(t, []string{inReview.ID}, mine(ports.OrderFilter{Status: domain.StatusPendingReview}), "filtra por estado")
	assert.Empty(t, mine(ports.OrderFilter{DeliveryDate: "2031-07-15", Status: domain.StatusPendingReview}), "los filtros se combinan")
}
