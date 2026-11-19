package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// PaymentAttemptRepository is the PostgreSQL implementation of ports.PaymentAttemptRepository.
type PaymentAttemptRepository struct {
	pool *pgxpool.Pool
}

var _ ports.PaymentAttemptRepository = (*PaymentAttemptRepository)(nil)

func NewPaymentAttemptRepository(pool *pgxpool.Pool) *PaymentAttemptRepository {
	return &PaymentAttemptRepository{pool: pool}
}

const paymentAttemptColumns = `id::text, customer_id::text, amount, method, status, reference, created_at`

func (r *PaymentAttemptRepository) SavePaymentAttempt(ctx context.Context, a domain.PaymentAttempt) (domain.PaymentAttempt, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO payment_attempts (customer_id, amount, method, status, reference, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+paymentAttemptColumns,
		a.CustomerID, a.Amount, string(a.Method), string(a.Status), a.Reference, a.CreatedAt)
	saved, err := scanPaymentAttempt(row.Scan)
	if err != nil {
		return domain.PaymentAttempt{}, fmt.Errorf("save payment attempt: %w", err)
	}
	return saved, nil
}

// ListPaymentAttempts returns the customer's attempts, newest first.
func (r *PaymentAttemptRepository) ListPaymentAttempts(ctx context.Context, customerID string) ([]domain.PaymentAttempt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+paymentAttemptColumns+`
		  FROM payment_attempts
		 WHERE customer_id = $1
		 ORDER BY created_at DESC, id ASC`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list payment attempts: %w", err)
	}
	defer rows.Close()

	out := []domain.PaymentAttempt{}
	for rows.Next() {
		a, err := scanPaymentAttempt(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list payment attempts: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list payment attempts: %w", err)
	}
	return out, nil
}

func scanPaymentAttempt(scan func(...any) error) (domain.PaymentAttempt, error) {
	var (
		a              domain.PaymentAttempt
		method, status string
	)
	if err := scan(&a.ID, &a.CustomerID, &a.Amount, &method, &status, &a.Reference, &a.CreatedAt); err != nil {
		return domain.PaymentAttempt{}, err
	}
	a.Method = domain.PaymentMethod(method)
	a.Status = domain.PaymentStatus(status)
	a.CreatedAt = a.CreatedAt.UTC()
	return a, nil
}
