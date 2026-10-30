package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// BalanceRepository is the PostgreSQL implementation of ports.BalanceRepository.
type BalanceRepository struct {
	pool *pgxpool.Pool
}

var _ ports.BalanceRepository = (*BalanceRepository)(nil)

func NewBalanceRepository(pool *pgxpool.Pool) *BalanceRepository {
	return &BalanceRepository{pool: pool}
}

// ListActiveObligations lists what still counts against an ACTIVO customer:
//   - a surcharge incident still PENDING (not yet priced into the next order). Its clock started when
//     the system confirmed it: the operator's verification time for a WRONG_ADDRESS, else its creation.
//   - a recidivism charge, while the customer's balance is above zero (its clock starts when applied).
//
// WAIVED incidents, those waiting for verification and APPLIED or PAID ones are not obligations.
func (r *BalanceRepository) ListActiveObligations(ctx context.Context) ([]domain.BalanceObligation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.customer_id::text, COALESCE(s.resolved_at, s.created_at)
		  FROM surcharges s
		  JOIN customers c ON c.id = s.customer_id
		 WHERE c.status = 'ACTIVO' AND s.status = 'PENDING'
		UNION ALL
		SELECT rc.customer_id::text, rc.created_at
		  FROM recidivism_charges rc
		  JOIN customers c ON c.id = rc.customer_id
		 WHERE c.status = 'ACTIVO' AND c.pending_surcharge > 0`)
	if err != nil {
		return nil, fmt.Errorf("list active obligations: %w", err)
	}
	defer rows.Close()

	out := []domain.BalanceObligation{}
	for rows.Next() {
		var o domain.BalanceObligation
		var at time.Time
		if err := rows.Scan(&o.CustomerID, &at); err != nil {
			return nil, fmt.Errorf("list active obligations: %w", err)
		}
		o.StartedAt = at.UTC()
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active obligations: %w", err)
	}
	return out, nil
}

// BlockCustomersForBalance changes only customers that are still ACTIVO.
func (r *BalanceRepository) BlockCustomersForBalance(ctx context.Context, customerIDs []string) (int, error) {
	if len(customerIDs) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE customers
		   SET status = 'BLOQUEADO_POR_SALDO', updated_at = now()
		 WHERE id = ANY($1::uuid[]) AND status = 'ACTIVO'`, customerIDs)
	if err != nil {
		return 0, fmt.Errorf("block customers for balance: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
