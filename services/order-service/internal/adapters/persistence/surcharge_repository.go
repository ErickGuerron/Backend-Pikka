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

// SurchargeRepository is the PostgreSQL implementation of ports.SurchargeRepository.
type SurchargeRepository struct {
	pool *pgxpool.Pool
}

var _ ports.SurchargeRepository = (*SurchargeRepository)(nil)

func NewSurchargeRepository(pool *pgxpool.Pool) *SurchargeRepository {
	return &SurchargeRepository{pool: pool}
}

const surchargeColumns = `id::text, customer_id::text, order_id::text, reason, status, amount, created_at,
	COALESCE(resolved_by::text, ''), COALESCE(resolution_reason, ''), resolved_at`

func (r *SurchargeRepository) RecordSurchargeIncident(ctx context.Context, s domain.Surcharge) (domain.Surcharge, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Surcharge{}, false, fmt.Errorf("record surcharge incident: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := scanSurcharge(tx.QueryRow(ctx, `
		INSERT INTO surcharges (customer_id, order_id, reason, status, amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (order_id) DO NOTHING
		RETURNING `+surchargeColumns,
		s.CustomerID, s.OrderID, s.Reason, string(s.Status), s.Amount, s.CreatedAt).Scan)
	if err == nil {
		// A priced PENDING incident is owed from this moment, in the same transaction as its row. One that
		// waits for verification is added when the operator confirms it.
		if err := addToBalance(ctx, tx, saved); err != nil {
			return domain.Surcharge{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.Surcharge{}, false, fmt.Errorf("record surcharge incident: commit: %w", err)
		}
		return saved, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Surcharge{}, false, fmt.Errorf("record surcharge incident: %w", err)
	}
	existing, err := scanSurcharge(tx.QueryRow(ctx,
		`SELECT `+surchargeColumns+` FROM surcharges WHERE order_id = $1`, s.OrderID).Scan)
	if err != nil {
		return domain.Surcharge{}, false, fmt.Errorf("record surcharge incident: load existing: %w", err)
	}
	return existing, false, nil
}

// addToBalance raises customers.pending_surcharge by the surcharge's amount when it is owed: PENDING and
// priced. Anything else (waiting for verification, waived, unpriced) changes nothing.
func addToBalance(ctx context.Context, tx pgx.Tx, s domain.Surcharge) error {
	if s.Status != domain.SurchargePending || !s.Amount.Valid {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customers SET pending_surcharge = pending_surcharge + $2, updated_at = now() WHERE id = $1`,
		s.CustomerID, s.Amount.Decimal); err != nil {
		return fmt.Errorf("add surcharge to balance: %w", err)
	}
	return nil
}

// OrderBasePrice sums the order's lines. found is false for an order without lines.
func (r *SurchargeRepository) OrderBasePrice(ctx context.Context, orderID string) (decimal.Decimal, bool, error) {
	var base decimal.Decimal
	var lines int
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(unit_price * quantity), 0), COUNT(*) FROM order_items WHERE order_id = $1`, orderID).Scan(&base, &lines); err != nil {
		return decimal.Zero, false, fmt.Errorf("order base price: %w", err)
	}
	return base, lines > 0, nil
}

// ResolveSurchargeVerification runs decide on the locked row and stores the audit it returns.
func (r *SurchargeRepository) ResolveSurchargeVerification(ctx context.Context, id string, decide func(domain.Surcharge) (domain.Surcharge, error)) (domain.Surcharge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Surcharge{}, fmt.Errorf("resolve surcharge: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanSurcharge(tx.QueryRow(ctx,
		`SELECT `+surchargeColumns+` FROM surcharges WHERE id = $1 FOR UPDATE`, id).Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Surcharge{}, domain.ErrSurchargeNotFound
	}
	if err != nil {
		return domain.Surcharge{}, fmt.Errorf("resolve surcharge: load: %w", err)
	}
	next, err := decide(current)
	if err != nil {
		return domain.Surcharge{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE surcharges
		   SET status = $2, resolved_by = $3, resolution_reason = $4, resolved_at = $5
		 WHERE id = $1`,
		id, string(next.Status), next.ResolvedBy, next.ResolutionReason, next.ResolvedAt); err != nil {
		return domain.Surcharge{}, fmt.Errorf("resolve surcharge: update: %w", err)
	}
	// Confirming a priced incident makes it owed now, in the same transaction as the decision.
	if current.Status == domain.SurchargePendingVerification {
		if err := addToBalance(ctx, tx, next); err != nil {
			return domain.Surcharge{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Surcharge{}, fmt.Errorf("resolve surcharge: commit: %w", err)
	}
	return next, nil
}

// ListSurchargesPendingVerification returns the waiting incidents, oldest first.
func (r *SurchargeRepository) ListSurchargesPendingVerification(ctx context.Context) ([]domain.Surcharge, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+surchargeColumns+`
		  FROM surcharges
		 WHERE status = 'PENDING_VERIFICATION'
		 ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list surcharges pending verification: %w", err)
	}
	defer rows.Close()

	out := []domain.Surcharge{}
	for rows.Next() {
		s, err := scanSurcharge(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list surcharges pending verification: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list surcharges pending verification: %w", err)
	}
	return out, nil
}

// CountedIncidentTimes returns the creation time of the customer's incidents that count for
// reincidence, oldest first. WAIVED and PENDING_VERIFICATION do not count.
func (r *SurchargeRepository) CountedIncidentTimes(ctx context.Context, customerID string) ([]time.Time, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT created_at
		  FROM surcharges
		 WHERE customer_id = $1 AND status IN ('PENDING', 'APPLIED', 'PAID')
		 ORDER BY created_at ASC`, customerID)
	if err != nil {
		return nil, fmt.Errorf("counted incident times: %w", err)
	}
	defer rows.Close()

	out := []time.Time{}
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return nil, fmt.Errorf("counted incident times: %w", err)
		}
		out = append(out, at.UTC())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("counted incident times: %w", err)
	}
	return out, nil
}

// MarkCustomerRecidivist sets customers.recidivist_at once; a customer already marked keeps its date.
func (r *SurchargeRepository) MarkCustomerRecidivist(ctx context.Context, customerID string, at time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE customers SET recidivist_at = $2 WHERE id = $1 AND recidivist_at IS NULL`, customerID, at.UTC())
	if err != nil {
		return false, fmt.Errorf("mark customer recidivist: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func scanSurcharge(scan func(...any) error) (domain.Surcharge, error) {
	var (
		s          domain.Surcharge
		status     string
		resolvedAt *time.Time
	)
	if err := scan(&s.ID, &s.CustomerID, &s.OrderID, &s.Reason, &status, &s.Amount, &s.CreatedAt,
		&s.ResolvedBy, &s.ResolutionReason, &resolvedAt); err != nil {
		return domain.Surcharge{}, err
	}
	s.Status = domain.SurchargeStatus(status)
	s.CreatedAt = s.CreatedAt.UTC()
	if resolvedAt != nil {
		s.ResolvedAt = resolvedAt.UTC()
	}
	return s, nil
}
