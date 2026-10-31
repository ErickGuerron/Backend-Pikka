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

// ListRecidivists returns the marked customers, oldest mark first. The contact is the phone of the latest
// order that has one, else the customer's own. Nothing else about the customer leaves the database.
func (r *RecidivismRepository) ListRecidivists(ctx context.Context) ([]domain.Recidivist, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id::text, c.full_name,
		       COALESCE(
		         (SELECT o.contact_phone FROM orders o
		           WHERE o.customer_id = c.id AND o.contact_phone <> ''
		           ORDER BY o.created_at DESC LIMIT 1),
		         c.phone, ''),
		       c.recidivist_at, c.pending_surcharge,
		       EXISTS (SELECT 1 FROM recidivism_charges rc
		                WHERE rc.customer_id = c.id AND rc.recidivist_at = c.recidivist_at)
		  FROM customers c
		 WHERE c.recidivist_at IS NOT NULL
		 ORDER BY c.recidivist_at, c.id`)
	if err != nil {
		return nil, fmt.Errorf("list recidivists: %w", err)
	}
	defer rows.Close()

	var out []domain.Recidivist
	for rows.Next() {
		var rec domain.Recidivist
		if err := rows.Scan(&rec.CustomerID, &rec.FullName, &rec.ContactPhone, &rec.RecidivistAt, &rec.PendingSurcharge, &rec.ChargeApplied); err != nil {
			return nil, fmt.Errorf("list recidivists: scan: %w", err)
		}
		rec.RecidivistAt = rec.RecidivistAt.UTC()
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recidivists: %w", err)
	}
	return out, nil
}
