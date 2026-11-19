package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ClearRecidivistMarkCommand is the administrator's decision to remove a customer's reincidence mark.
type ClearRecidivistMarkCommand struct {
	Role       string
	AdminID    string
	CustomerID string
	Reason     string
}

// ClearRecidivistMark removes the reincidence mark with an audited reason. Only an administrator may do it
// (ADR 0006, D-F5b): an operator can charge a recidivist but cannot erase the history that allows it.
type ClearRecidivistMark struct {
	repo  ports.RecidivismRepository
	clock func() time.Time
}

func NewClearRecidivistMark(repo ports.RecidivismRepository) *ClearRecidivistMark {
	return &ClearRecidivistMark{repo: repo, clock: time.Now}
}

func (uc *ClearRecidivistMark) Run(ctx context.Context, cmd ClearRecidivistMarkCommand) (domain.RecidivistClear, error) {
	if cmd.Role != RoleAdmin {
		return domain.RecidivistClear{}, domain.ErrNotAdmin
	}
	customerID, err := uuid.Parse(cmd.CustomerID)
	if err != nil {
		return domain.RecidivistClear{}, domain.ErrCustomerNotFound
	}
	// Same validation as under the lock, with a zero mark, so a bad reason never reaches the database.
	if _, err := domain.NewRecidivistClear(customerID.String(), time.Time{}, cmd.AdminID, cmd.Reason, uc.clock()); err != nil {
		return domain.RecidivistClear{}, err
	}
	return uc.repo.ClearRecidivistMark(ctx, customerID.String(), func(recidivistAt time.Time) (domain.RecidivistClear, error) {
		return domain.NewRecidivistClear(customerID.String(), recidivistAt, cmd.AdminID, cmd.Reason, uc.clock())
	})
}
