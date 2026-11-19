package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ApplyRecidivismChargeCommand is the operator's decision about a recidivist customer. Amount is a
// decimal string so no float ever touches money.
type ApplyRecidivismChargeCommand struct {
	Role       string
	OperatorID string
	CustomerID string
	Amount     string
	Reason     string
}

// ApplyRecidivismCharge lets an operator or admin charge a recidivist an extra amount, with an
// audited reason (ADR 0006, section 1).
type ApplyRecidivismCharge struct {
	repo  ports.RecidivismRepository
	max   decimal.Decimal
	clock func() time.Time
}

// NewApplyRecidivismCharge limits every charge to max. A max that is not positive rejects every charge:
// a missing limit must never mean "no limit".
func NewApplyRecidivismCharge(repo ports.RecidivismRepository, max decimal.Decimal) *ApplyRecidivismCharge {
	return &ApplyRecidivismCharge{repo: repo, max: max, clock: time.Now}
}

// Run validates everything it can before the repository and builds the charge under its lock.
func (uc *ApplyRecidivismCharge) Run(ctx context.Context, cmd ApplyRecidivismChargeCommand) (domain.RecidivismCharge, error) {
	if !isOperator(cmd.Role) {
		return domain.RecidivismCharge{}, domain.ErrNotOperator
	}
	customerID, err := uuid.Parse(cmd.CustomerID)
	if err != nil {
		return domain.RecidivismCharge{}, domain.ErrCustomerNotFound
	}
	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil {
		return domain.RecidivismCharge{}, domain.ErrInvalidRecidivismCharge
	}
	// The same validation runs inside build with the real mark; running it here with a zero mark
	// rejects a bad amount, operator or reason without touching the database.
	if _, err := domain.NewRecidivismCharge(customerID.String(), time.Time{}, amount, cmd.OperatorID, cmd.Reason, uc.clock()); err != nil {
		return domain.RecidivismCharge{}, err
	}
	if amount.Round(2).GreaterThan(uc.max) || !uc.max.IsPositive() {
		return domain.RecidivismCharge{}, fmt.Errorf("%w: %s", domain.ErrRecidivismChargeTooHigh, uc.max.StringFixed(2))
	}
	return uc.repo.ApplyRecidivismCharge(ctx, customerID.String(), func(recidivistAt time.Time) (domain.RecidivismCharge, error) {
		return domain.NewRecidivismCharge(customerID.String(), recidivistAt, amount, cmd.OperatorID, cmd.Reason, uc.clock())
	})
}
