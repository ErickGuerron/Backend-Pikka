package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RecidivismRepository is the PostgreSQL implementation of ports.RecidivismRepository.
type RecidivismRepository struct {
	pool *pgxpool.Pool
}

var _ ports.RecidivismRepository = (*RecidivismRepository)(nil)

func NewRecidivismRepository(pool *pgxpool.Pool) *RecidivismRepository {
	return &RecidivismRepository{pool: pool}
}

func (r *RecidivismRepository) ApplyRecidivismCharge(ctx context.Context, customerID string, build func(time.Time) (domain.RecidivismCharge, error)) (domain.RecidivismCharge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.RecidivismCharge{}, fmt.Errorf("apply recidivism charge: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var markedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT recidivist_at FROM customers WHERE id = $1 FOR UPDATE`, customerID).Scan(&markedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecidivismCharge{}, domain.ErrCustomerNotFound
	}
	if err != nil {
		return domain.RecidivismCharge{}, fmt.Errorf("apply recidivism charge: load customer: %w", err)
	}
	if markedAt == nil {
		return domain.RecidivismCharge{}, domain.ErrCustomerNotRecidivist
	}

	charge, err := build(markedAt.UTC())
	if err != nil {
		return domain.RecidivismCharge{}, err
	}
	saved := charge
	err = tx.QueryRow(ctx, `
		INSERT INTO recidivism_charges (customer_id, recidivist_at, amount, reason, applied_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text`,
		charge.CustomerID, charge.RecidivistAt, charge.Amount, charge.Reason, charge.AppliedBy, charge.CreatedAt).Scan(&saved.ID)
	if isUniqueViolation(err) {
		return domain.RecidivismCharge{}, domain.ErrRecidivismChargeExists
	}
	if err != nil {
		return domain.RecidivismCharge{}, fmt.Errorf("apply recidivism charge: insert: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customers SET pending_surcharge = pending_surcharge + $2, updated_at = now() WHERE id = $1`,
		customerID, charge.Amount); err != nil {
		return domain.RecidivismCharge{}, fmt.Errorf("apply recidivism charge: add to balance: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.RecidivismCharge{}, fmt.Errorf("apply recidivism charge: commit: %w", err)
	}
	return saved, nil
}
