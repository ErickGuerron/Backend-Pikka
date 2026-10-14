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

// Requiere ORDER_TEST_DATABASE_URL apuntando al esquema orders.
func TestOrderReviewAndNotificationsPersistTogether(t *testing.T) {
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

	customer, err := NewCustomerRepository(pool).Create(ctx, domain.Customer{
		UserID:           uuid.New(),
		Email:            fmt.Sprintf("revision-%s@test.local", uuid.NewString()),
		FullName:         "Cliente de revisión",
		Status:           domain.StatusActive,
		PendingSurcharge: decimal.Zero,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM customer_notifications WHERE customer_id = $1`, customer.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE customer_id = $1`, customer.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE id = $1`, customer.ID)
	})

	repo := NewOrderRepository(pool)
	order, err := domain.NewOrder(domain.NewOrderInput{
		Address:      "Dirección sin localizar",
		DeliveryDate: "2030-04-12",
		WindowStart:  "09:00",
		WindowEnd:    "12:00",
	}, time.Date(2030, 4, 10, 15, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	order.CustomerID = customer.ID.String()
	order, err = order.ApplyValidation([]domain.ReviewReason{domain.ReviewAddressNotLocated})
	require.NoError(t, err)

	inReview := domain.NewNotification(customer.ID.String(), "", domain.NotificationInReview, "ADDRESS_NOT_LOCATED")
	created, err := repo.Create(ctx, order, &inReview)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPendingReview, created.Status)

	exceptions, err := repo.ListExceptions(ctx)
	require.NoError(t, err)
	require.Contains(t, orderIDs(exceptions), created.ID, "el pedido en revisión aparece en la cola")

	// El operador rechaza: el estado y el aviso se guardan en la misma transacción.
	rejected, err := repo.Review(ctx, created.ID, func(current domain.Order) (domain.Order, *domain.Notification, error) {
		next, err := current.Reject()
		if err != nil {
			return domain.Order{}, nil, err
		}
		n := domain.NewNotification(current.CustomerID, current.ID, domain.NotificationRejected, "Dirección imposible de ubicar")
		return next, &n, nil
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusCancelled, rejected.Status)
	require.Empty(t, rejected.ReviewReasons)

	exceptions, err = repo.ListExceptions(ctx)
	require.NoError(t, err)
	require.NotContains(t, orderIDs(exceptions), created.ID, "un pedido cancelado sale de la cola")

	notifications, err := NewNotificationRepository(pool).ListByCustomer(ctx, customer.ID)
	require.NoError(t, err)
	require.Len(t, notifications, 2)
	require.Equal(t, domain.NotificationRejected, notifications[0].Kind, "el aviso más reciente primero")
	require.Equal(t, "Dirección imposible de ubicar", notifications[0].Detail)
	require.Equal(t, domain.NotificationInReview, notifications[1].Kind)

	_, err = repo.Review(ctx, uuid.NewString(), func(current domain.Order) (domain.Order, *domain.Notification, error) {
		return current, nil, nil
	})
	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func orderIDs(orders []domain.Order) []string {
	ids := make([]string, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.ID)
	}
	return ids
}
