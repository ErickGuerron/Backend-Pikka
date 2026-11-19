package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// DisputeRepository is the PostgreSQL implementation of ports.DisputeRepository.
type DisputeRepository struct {
	pool *pgxpool.Pool
}

var _ ports.DisputeRepository = (*DisputeRepository)(nil)

func NewDisputeRepository(pool *pgxpool.Pool) *DisputeRepository {
	return &DisputeRepository{pool: pool}
}

const disputeColumns = `id::text, surcharge_id::text, customer_id::text, reason, status, opened_at,
	COALESCE(resolved_by::text, ''), COALESCE(resolution_reason, ''), resolved_at`

func scanDispute(scan func(...any) error) (domain.Dispute, error) {
	var (
		d          domain.Dispute
		status     string
		resolvedAt *time.Time
	)
	if err := scan(&d.ID, &d.SurchargeID, &d.CustomerID, &d.Reason, &status, &d.OpenedAt, &d.ResolvedBy, &d.ResolutionReason, &resolvedAt); err != nil {
		return domain.Dispute{}, err
	}
	d.Status = domain.DisputeStatus(status)
	d.OpenedAt = d.OpenedAt.UTC()
	if resolvedAt != nil {
		d.ResolvedAt = resolvedAt.UTC()
	}
	return d, nil
}

func (r *DisputeRepository) OpenDispute(ctx context.Context, surchargeID, customerID string, build func(domain.Surcharge) (domain.Dispute, error)) (domain.Dispute, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Dispute{}, fmt.Errorf("open dispute: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	s, err := scanSurcharge(tx.QueryRow(ctx, `SELECT `+surchargeColumns+` FROM surcharges WHERE id = $1 FOR UPDATE`, surchargeID).Scan)
	// A surcharge of another customer answers like one that does not exist: nobody learns which ids are real.
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && s.CustomerID != customerID) {
		return domain.Dispute{}, domain.ErrSurchargeNotFound
	}
	if err != nil {
		return domain.Dispute{}, fmt.Errorf("open dispute: load surcharge: %w", err)
	}
	d, err := build(s)
	if err != nil {
		return domain.Dispute{}, err
	}
	saved := d
	err = tx.QueryRow(ctx, `
		INSERT INTO surcharge_disputes (surcharge_id, customer_id, reason, status, opened_at)
		VALUES ($1, $2, $3, 'OPEN', $4)
		RETURNING id::text`, d.SurchargeID, d.CustomerID, d.Reason, d.OpenedAt).Scan(&saved.ID)
	if isUniqueViolation(err) {
		return domain.Dispute{}, domain.ErrDisputeExists
	}
	if err != nil {
		return domain.Dispute{}, fmt.Errorf("open dispute: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Dispute{}, fmt.Errorf("open dispute: commit: %w", err)
	}
	return saved, nil
}

func (r *DisputeRepository) ResolveDispute(ctx context.Context, disputeID string, decide func(domain.Dispute) (domain.Dispute, error)) (domain.Dispute, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Dispute{}, fmt.Errorf("resolve dispute: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanDispute(tx.QueryRow(ctx, `SELECT `+disputeColumns+` FROM surcharge_disputes WHERE id = $1 FOR UPDATE`, disputeID).Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Dispute{}, domain.ErrDisputeNotFound
	}
	if err != nil {
		return domain.Dispute{}, fmt.Errorf("resolve dispute: load: %w", err)
	}
	next, err := decide(current)
	if err != nil {
		return domain.Dispute{}, err
	}
	if next.Status == domain.DisputeInFavor {
		if err := waiveForDispute(ctx, tx, next); err != nil {
			return domain.Dispute{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE surcharge_disputes
		   SET status = $2, resolved_by = $3, resolution_reason = $4, resolved_at = $5
		 WHERE id = $1`,
		disputeID, string(next.Status), next.ResolvedBy, next.ResolutionReason, next.ResolvedAt); err != nil {
		return domain.Dispute{}, fmt.Errorf("resolve dispute: update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Dispute{}, fmt.Errorf("resolve dispute: commit: %w", err)
	}
	return next, nil
}

// waiveForDispute waives the disputed surcharge and takes its amount off the customer's balance. The customer row is
// locked first, like a payment does, so a ruling and a payment cannot interleave. A surcharge that is no longer owed
// (it was paid meanwhile) cannot be waived: refunding it is out of scope.
func waiveForDispute(ctx context.Context, tx pgx.Tx, d domain.Dispute) error {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM customers WHERE id = $1 FOR UPDATE`, d.CustomerID); err != nil {
		return fmt.Errorf("resolve dispute: lock customer: %w", err)
	}
	var (
		status string
		amount decimal.NullDecimal
	)
	if err := tx.QueryRow(ctx, `SELECT status, amount FROM surcharges WHERE id = $1 FOR UPDATE`, d.SurchargeID).Scan(&status, &amount); err != nil {
		return fmt.Errorf("resolve dispute: load surcharge: %w", err)
	}
	if status != string(domain.SurchargePending) || !amount.Valid {
		return domain.ErrSurchargeNotDisputable
	}
	if _, err := tx.Exec(ctx, `UPDATE surcharges SET status = 'WAIVED' WHERE id = $1`, d.SurchargeID); err != nil {
		return fmt.Errorf("resolve dispute: waive: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customers SET pending_surcharge = pending_surcharge - $2, updated_at = now() WHERE id = $1`,
		d.CustomerID, amount.Decimal); err != nil {
		return fmt.Errorf("resolve dispute: take off the balance: %w", err)
	}
	return nil
}

func (r *DisputeRepository) ListOpenDisputes(ctx context.Context) ([]domain.Dispute, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+disputeColumns+`
		  FROM surcharge_disputes
		 WHERE status = 'OPEN'
		 ORDER BY opened_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list open disputes: %w", err)
	}
	defer rows.Close()

	out := []domain.Dispute{}
	for rows.Next() {
		d, err := scanDispute(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list open disputes: scan: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list open disputes: %w", err)
	}
	return out, nil
}
