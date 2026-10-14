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
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/infrastructure/postgres"
)

// Requiere ORDER_TEST_DATABASE_URL apuntando al esquema orders (ver README del servicio).
func TestOrderRepositoryPersistsAndListsByCustomer(t *testing.T) {
	dsn := os.Getenv("ORDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORDER_TEST_DATABASE_URL is not set")
	}
	// Esta prueba corre antes que las del paquete persistence_test: crea el esquema por su cuenta.
	require.NoError(t, postgres.Migrate(dsn))
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	customers := NewCustomerRepository(pool)
	customer, err := customers.Create(ctx, domain.Customer{
		UserID:           uuid.New(),
		Email:            fmt.Sprintf("cliente-%s@test.local", uuid.NewString()),
		FullName:         "Cliente de prueba",
		Status:           domain.StatusActive,
		PendingSurcharge: decimal.Zero,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE customer_id = $1`, customer.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE id = $1`, customer.ID)
	})

	repo := NewOrderRepository(pool)
	registeredAt := time.Date(2030, 4, 10, 15, 0, 0, 0, time.UTC)
	newOrder := func(zoneID string, reasons ...domain.ReviewReason) domain.Order {
		t.Helper()
		order, err := domain.NewOrder(domain.NewOrderInput{
			Address:      "Avenida Amazonas 123",
			Latitude:     -0.18,
			Longitude:    -78.48,
			DeliveryDate: "2030-04-12",
			WindowStart:  "09:00",
			WindowEnd:    "12:00",
		}, registeredAt)
		require.NoError(t, err)
		order.CustomerID = customer.ID.String()
		order.ZoneID = zoneID
		order, err = order.ApplyValidation(reasons)
		require.NoError(t, err)
		return order
	}

	zoneID := uuid.NewString()
	covered, err := repo.Create(ctx, newOrder(zoneID), nil)
	require.NoError(t, err)
	require.NotEmpty(t, covered.ID)
	require.Equal(t, domain.StatusPending, covered.Status)

	notice := domain.NewNotification(customer.ID.String(), "", domain.NotificationInReview, "ZONE_NOT_COVERED")
	outside, err := repo.Create(ctx, newOrder("", domain.ReviewZoneNotCovered), &notice)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPendingReview, outside.Status)

	orders, err := repo.ListByCustomer(ctx, customer.ID)
	require.NoError(t, err)
	require.Len(t, orders, 2)

	// Más recientes primero: el segundo pedido se creó después del primero.
	got := orders[0]
	require.Equal(t, outside.ID, got.ID)
	require.Equal(t, domain.StatusPendingReview, got.Status)
	require.Equal(t, []domain.ReviewReason{domain.ReviewZoneNotCovered}, got.ReviewReasons)
	require.Empty(t, got.ZoneID, "fuera de zona no hay zone_id")
	require.Equal(t, "2030-04-12", got.DeliveryDate.String())
	require.Equal(t, "09:00", got.Window.Start())
	require.Equal(t, "12:00", got.Window.End())

	require.Equal(t, covered.ID, orders[1].ID)
	require.Equal(t, zoneID, orders[1].ZoneID)
	require.Empty(t, orders[1].ReviewReasons)

	// Un cliente sin pedidos recibe una lista vacía, no un error.
	none, err := repo.ListByCustomer(ctx, uuid.New())
	require.NoError(t, err)
	require.Empty(t, none)
}
