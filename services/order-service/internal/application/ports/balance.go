package ports

import (
	"context"

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
}
