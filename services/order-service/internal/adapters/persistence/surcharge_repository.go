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
	saved, err := scanSurcharge(r.pool.QueryRow(ctx, `
		INSERT INTO surcharges (customer_id, order_id, reason, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (order_id) DO NOTHING
		RETURNING `+surchargeColumns,
		s.CustomerID, s.OrderID, s.Reason, string(s.Status), s.CreatedAt).Scan)
	if err == nil {
		return saved, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Surcharge{}, false, fmt.Errorf("record surcharge incident: %w", err)
	}
	existing, err := scanSurcharge(r.pool.QueryRow(ctx,
		`SELECT `+surchargeColumns+` FROM surcharges WHERE order_id = $1`, s.OrderID).Scan)
	if err != nil {
		return domain.Surcharge{}, false, fmt.Errorf("record surcharge incident: load existing: %w", err)
	}
	return existing, false, nil
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
