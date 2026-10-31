package ports

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// BalanceRepository is what the overdue-balance sweep needs (ADR 0006, section 5).
type BalanceRepository interface {
	// ListActiveObligations returns the unpaid obligations of customers that are still ACTIVO: surcharge
	// incidents waiting for the next order and recidivism charges still in the balance. The two-week
	// rule is applied by the caller, not here.
	ListActiveObligations(ctx context.Context) ([]domain.BalanceObligation, error)
	// BlockCustomersForBalance moves the given customers from ACTIVO to BLOQUEADO_POR_SALDO and returns
	// how many changed. A customer in any other status is left alone, so a block for incidents is never
	// overwritten.
	BlockCustomersForBalance(ctx context.Context, customerIDs []string) (int, error)
	// ListOwedLines returns what the customer owes, oldest first: priced PENDING surcharges and unpaid
	// recidivism charges. Waived, waiting for verification, paid and unpriced ones are not owed.
	ListOwedLines(ctx context.Context, customerID string) ([]domain.BalanceLine, error)
	// SettleBalance records the full payment in one transaction: it locks the customer, checks that the
	// balance is still exactly amount (else ErrBalanceChanged), stamps the priced PENDING or APPLIED
	// surcharges as PAID and the unpaid recidivism charges as paid, zeroes the balance and, if the account
	// was BLOQUEADO_POR_SALDO, returns it to ACTIVO. It reports whether it unblocked the account. A block
	// for incidents is never lifted by a payment.
	SettleBalance(ctx context.Context, customerID string, amount decimal.Decimal, at time.Time) (unblocked bool, err error)
}
