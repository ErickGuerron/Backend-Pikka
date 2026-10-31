package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ReassignmentRepository is the PostgreSQL implementation of ports.ReassignmentRepository.
type ReassignmentRepository struct {
	pool *pgxpool.Pool
}

var _ ports.ReassignmentRepository = (*ReassignmentRepository)(nil)

func NewReassignmentRepository(pool *pgxpool.Pool) *ReassignmentRepository {
	return &ReassignmentRepository{pool: pool}
}

func (r *ReassignmentRepository) SetReassignmentFee(ctx context.Context, fee domain.ReassignmentFee) (domain.ReassignmentFee, error) {
	saved := fee
	err := r.pool.QueryRow(ctx, `
		INSERT INTO reassignment_fees (amount, reason, set_by, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`, fee.Amount, fee.Reason, fee.SetBy, fee.CreatedAt).Scan(&saved.ID)
	if err != nil {
		return domain.ReassignmentFee{}, fmt.Errorf("set reassignment fee: %w", err)
	}
	return saved, nil
}

func (r *ReassignmentRepository) CurrentReassignmentFee(ctx context.Context) (domain.ReassignmentFee, error) {
	return currentReassignmentFee(ctx, r.pool)
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func currentReassignmentFee(ctx context.Context, q rowQuerier) (domain.ReassignmentFee, error) {
	var fee domain.ReassignmentFee
	err := q.QueryRow(ctx, `
		SELECT id::text, amount, reason, set_by::text, created_at
		  FROM reassignment_fees ORDER BY created_at DESC, id DESC LIMIT 1`).
		Scan(&fee.ID, &fee.Amount, &fee.Reason, &fee.SetBy, &fee.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReassignmentFee{}, domain.ErrReassignmentFeeNotSet
	}
	if err != nil {
		return domain.ReassignmentFee{}, fmt.Errorf("current reassignment fee: %w", err)
	}
	fee.CreatedAt = fee.CreatedAt.UTC()
	return fee, nil
}

func (r *ReassignmentRepository) ApplyReassignmentCharge(ctx context.Context, orderID string, build func(string, *domain.ReassignmentFee) (domain.ReassignmentCharge, error)) (domain.ReassignmentCharge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ReassignmentCharge{}, fmt.Errorf("apply reassignment charge: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var customerID string
	err = tx.QueryRow(ctx, `SELECT customer_id::text FROM orders WHERE id = $1`, orderID).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReassignmentCharge{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.ReassignmentCharge{}, fmt.Errorf("apply reassignment charge: load order: %w", err)
	}
	// Lock the customer first, like a payment does, so a charge and a payment cannot interleave.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM customers WHERE id = $1 FOR UPDATE`, customerID); err != nil {
		return domain.ReassignmentCharge{}, fmt.Errorf("apply reassignment charge: lock customer: %w", err)
	}

	var fee *domain.ReassignmentFee
	current, err := currentReassignmentFee(ctx, tx)
	switch {
	case err == nil:
		fee = &current
	case !errors.Is(err, domain.ErrReassignmentFeeNotSet):
		return domain.ReassignmentCharge{}, err
	}
	charge, err := build(customerID, fee)
	if err != nil {
		return domain.ReassignmentCharge{}, err
	}
	saved := charge
	err = tx.QueryRow(ctx, `
		INSERT INTO reassignment_charges (customer_id, order_id, amount, reason, applied_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text`,
		charge.CustomerID, charge.OrderID, charge.Amount, charge.Reason, charge.AppliedBy, charge.CreatedAt).Scan(&saved.ID)
	if isUniqueViolation(err) {
		return domain.ReassignmentCharge{}, domain.ErrReassignmentChargeExists
	}
	if err != nil {
		return domain.ReassignmentCharge{}, fmt.Errorf("apply reassignment charge: insert: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customers SET pending_surcharge = pending_surcharge + $2, updated_at = now() WHERE id = $1`,
		customerID, charge.Amount); err != nil {
		return domain.ReassignmentCharge{}, fmt.Errorf("apply reassignment charge: add to balance: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ReassignmentCharge{}, fmt.Errorf("apply reassignment charge: commit: %w", err)
	}
	return saved, nil
}
