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

// BalanceRepository is the PostgreSQL implementation of ports.BalanceRepository.
type BalanceRepository struct {
	pool *pgxpool.Pool
}

var _ ports.BalanceRepository = (*BalanceRepository)(nil)

func NewBalanceRepository(pool *pgxpool.Pool) *BalanceRepository {
	return &BalanceRepository{pool: pool}
}

// ListActiveObligations lists what still counts against an ACTIVO customer:
//   - a priced surcharge incident still PENDING (owed, not paid; one without amount cannot be paid, so it
//     never blocks). Its clock started when
//     the system confirmed it: the operator's verification time for a WRONG_ADDRESS, else its creation.
//   - a recidivism charge not yet paid (its clock starts when applied).
//   - a reassignment charge not yet paid (its clock starts when applied).
//
// WAIVED incidents, those waiting for verification and APPLIED or PAID ones are not obligations.
// A recidivism charge stops being one when it is stamped paid.
func (r *BalanceRepository) ListActiveObligations(ctx context.Context) ([]domain.BalanceObligation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.customer_id::text, COALESCE(s.resolved_at, s.created_at)
		  FROM surcharges s
		  JOIN customers c ON c.id = s.customer_id
		 WHERE c.status = 'ACTIVO' AND s.status = 'PENDING' AND s.amount IS NOT NULL
		UNION ALL
		SELECT rc.customer_id::text, rc.created_at
		  FROM recidivism_charges rc
		  JOIN customers c ON c.id = rc.customer_id
		 WHERE c.status = 'ACTIVO' AND rc.paid_at IS NULL
		UNION ALL
		SELECT ra.customer_id::text, ra.created_at
		  FROM reassignment_charges ra
		  JOIN customers c ON c.id = ra.customer_id
		 WHERE c.status = 'ACTIVO' AND ra.paid_at IS NULL`)
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

// SettleBalance applies a full payment atomically. The customer row is locked first, so a payment and a
// new surcharge cannot interleave: the balance is compared with what was charged under the lock.
func (r *BalanceRepository) SettleBalance(ctx context.Context, customerID string, amount decimal.Decimal, at time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("settle balance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var owed decimal.Decimal
	err = tx.QueryRow(ctx, `SELECT status, pending_surcharge FROM customers WHERE id = $1 FOR UPDATE`, customerID).Scan(&status, &owed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrCustomerNotFound
	}
	if err != nil {
		return false, fmt.Errorf("settle balance: load customer: %w", err)
	}
	if !owed.Equal(amount) {
		return false, domain.ErrBalanceChanged
	}

	// Only priced incidents are paid: one without amount was never added to the balance.
	if _, err := tx.Exec(ctx, `
		UPDATE surcharges SET status = 'PAID', paid_at = $2
		 WHERE customer_id = $1 AND status IN ('PENDING', 'APPLIED') AND amount IS NOT NULL`, customerID, at.UTC()); err != nil {
		return false, fmt.Errorf("settle balance: surcharges: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE recidivism_charges SET paid_at = $2 WHERE customer_id = $1 AND paid_at IS NULL`, customerID, at.UTC()); err != nil {
		return false, fmt.Errorf("settle balance: recidivism charges: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE reassignment_charges SET paid_at = $2 WHERE customer_id = $1 AND paid_at IS NULL`, customerID, at.UTC()); err != nil {
		return false, fmt.Errorf("settle balance: reassignment charges: %w", err)
	}
	unblocked := status == string(domain.StatusBlockedSurcharge)
	if _, err := tx.Exec(ctx, `
		UPDATE customers
		   SET pending_surcharge = pending_surcharge - $2,
		       status = CASE WHEN status = 'BLOQUEADO_POR_SALDO' THEN 'ACTIVO' ELSE status END,
		       updated_at = now()
		 WHERE id = $1`, customerID, amount); err != nil {
		return false, fmt.Errorf("settle balance: customer: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("settle balance: commit: %w", err)
	}
	return unblocked, nil
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
