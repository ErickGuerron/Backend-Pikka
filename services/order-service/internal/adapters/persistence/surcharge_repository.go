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

// SurchargeRepository is the PostgreSQL implementation of ports.SurchargeRepository.
type SurchargeRepository struct {
	pool *pgxpool.Pool
}

var _ ports.SurchargeRepository = (*SurchargeRepository)(nil)

func NewSurchargeRepository(pool *pgxpool.Pool) *SurchargeRepository {
	return &SurchargeRepository{pool: pool}
}

const surchargeColumns = `id::text, customer_id::text, order_id::text, reason, status, amount, created_at`

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

func scanSurcharge(scan func(...any) error) (domain.Surcharge, error) {
	var (
		s      domain.Surcharge
		status string
	)
	if err := scan(&s.ID, &s.CustomerID, &s.OrderID, &s.Reason, &status, &s.Amount, &s.CreatedAt); err != nil {
		return domain.Surcharge{}, err
	}
	s.Status = domain.SurchargeStatus(status)
	s.CreatedAt = s.CreatedAt.UTC()
	return s, nil
}
