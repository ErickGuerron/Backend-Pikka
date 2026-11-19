package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// NotificationRepository es la implementación PostgreSQL de ports.NotificationRepository.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

var _ ports.NotificationRepository = (*NotificationRepository)(nil)

// NewNotificationRepository inyecta el pool pgx.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

// ListByCustomer devuelve los avisos del cliente, del más reciente al más antiguo.
func (r *NotificationRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, customer_id::text, COALESCE(order_id::text, ''), kind, detail, created_at, read_at IS NOT NULL
		   FROM customer_notifications
		  WHERE customer_id = $1
		  ORDER BY created_at DESC, id ASC`,
		customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	out := []domain.Notification{}
	for rows.Next() {
		var (
			n    domain.Notification
			kind string
			at   time.Time
		)
		if err := rows.Scan(&n.ID, &n.CustomerID, &n.OrderID, &kind, &n.Detail, &at, &n.Read); err != nil {
			return nil, fmt.Errorf("list notifications: %w", err)
		}
		n.Kind = domain.NotificationKind(kind)
		n.CreatedAt = at.UTC()
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return out, nil
}
