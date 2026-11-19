package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// SetReassignmentFeeCommand is the administrator's decision about the fixed fee. Amount is a decimal string.
type SetReassignmentFeeCommand struct {
	Role    string
	AdminID string
	Amount  string
	Reason  string
}

// SetReassignmentFee lets an administrator define or change the fixed reassignment fee, with an audited reason.
type SetReassignmentFee struct {
	repo  ports.ReassignmentRepository
	clock func() time.Time
}

func NewSetReassignmentFee(repo ports.ReassignmentRepository) *SetReassignmentFee {
	return &SetReassignmentFee{repo: repo, clock: time.Now}
}

func (uc *SetReassignmentFee) Run(ctx context.Context, cmd SetReassignmentFeeCommand) (domain.ReassignmentFee, error) {
	if cmd.Role != RoleAdmin {
		return domain.ReassignmentFee{}, domain.ErrNotAdmin
	}
	amount, err := decimal.NewFromString(cmd.Amount)
	if err != nil {
		return domain.ReassignmentFee{}, domain.ErrInvalidReassignmentFee
	}
	fee, err := domain.NewReassignmentFee(amount, cmd.AdminID, cmd.Reason, uc.clock())
	if err != nil {
		return domain.ReassignmentFee{}, err
	}
	return uc.repo.SetReassignmentFee(ctx, fee)
}

// GetReassignmentFee shows the current fee to the staff that applies it.
type GetReassignmentFee struct{ repo ports.ReassignmentRepository }

func NewGetReassignmentFee(repo ports.ReassignmentRepository) *GetReassignmentFee {
	return &GetReassignmentFee{repo: repo}
}

func (uc *GetReassignmentFee) Run(ctx context.Context, role string) (domain.ReassignmentFee, error) {
	if !isOperator(role) {
		return domain.ReassignmentFee{}, domain.ErrNotOperator
	}
	return uc.repo.CurrentReassignmentFee(ctx)
}

// ApplyReassignmentChargeCommand is the operator's decision to charge the fee for one order. There is no amount:
// it is always the current fee.
type ApplyReassignmentChargeCommand struct {
	Role       string
	OperatorID string
	OrderID    string
	Reason     string
}

// ApplyReassignmentCharge charges the current fixed fee to the customer of a reassigned order.
type ApplyReassignmentCharge struct {
	repo  ports.ReassignmentRepository
	clock func() time.Time
}

func NewApplyReassignmentCharge(repo ports.ReassignmentRepository) *ApplyReassignmentCharge {
	return &ApplyReassignmentCharge{repo: repo, clock: time.Now}
}

func (uc *ApplyReassignmentCharge) Run(ctx context.Context, cmd ApplyReassignmentChargeCommand) (domain.ReassignmentCharge, error) {
	if !isOperator(cmd.Role) {
		return domain.ReassignmentCharge{}, domain.ErrNotOperator
	}
	orderID, err := uuid.Parse(cmd.OrderID)
	if err != nil {
		return domain.ReassignmentCharge{}, domain.ErrOrderNotFound
	}
	// Same validation as under the lock, so a bad reason never reaches the database.
	if _, err := domain.ReviewDecisionReason(cmd.Reason); err != nil {
		return domain.ReassignmentCharge{}, err
	}
	return uc.repo.ApplyReassignmentCharge(ctx, orderID.String(), func(customerID string, fee *domain.ReassignmentFee) (domain.ReassignmentCharge, error) {
		return domain.NewReassignmentCharge(customerID, orderID.String(), fee, cmd.OperatorID, cmd.Reason, uc.clock())
	})
}
