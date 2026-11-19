package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// OpenDisputeCommand is a customer's objection to one surcharge. The customer is the verified identity, never a field
// of the request.
type OpenDisputeCommand struct {
	UserID      uuid.UUID
	Role        string
	SurchargeID string
	Reason      string
}

// OpenDispute lets a customer dispute one of their own surcharges inside the window (ADR 0007, section 6).
type OpenDispute struct {
	customers ports.CustomerRepository
	disputes  ports.DisputeRepository
	clock     func() time.Time
}

func NewOpenDispute(customers ports.CustomerRepository, disputes ports.DisputeRepository) *OpenDispute {
	return &OpenDispute{customers: customers, disputes: disputes, clock: time.Now}
}

func (uc *OpenDispute) Run(ctx context.Context, cmd OpenDisputeCommand) (domain.Dispute, error) {
	if cmd.Role != RoleCustomer {
		return domain.Dispute{}, domain.ErrNotCustomer
	}
	surchargeID, err := uuid.Parse(cmd.SurchargeID)
	if err != nil {
		return domain.Dispute{}, domain.ErrSurchargeNotFound
	}
	// The reason is validated again under the lock; checking it here keeps a bad one away from the database.
	if _, err := domain.ReviewDecisionReason(cmd.Reason); err != nil {
		return domain.Dispute{}, err
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return domain.Dispute{}, err
	}
	return uc.disputes.OpenDispute(ctx, surchargeID.String(), customer.ID.String(), func(s domain.Surcharge) (domain.Dispute, error) {
		return domain.NewDispute(s, cmd.Reason, uc.clock())
	})
}

// ResolveDisputeCommand is the staff's decision about an open dispute. Decision is IN_FAVOR or AGAINST.
type ResolveDisputeCommand struct {
	Role       string
	OperatorID string
	DisputeID  string
	Decision   string
	Reason     string
}

// ResolveDispute lets an operator or admin decide a dispute, with an audited reason.
type ResolveDispute struct {
	disputes ports.DisputeRepository
	clock    func() time.Time
}

func NewResolveDispute(disputes ports.DisputeRepository) *ResolveDispute {
	return &ResolveDispute{disputes: disputes, clock: time.Now}
}

func (uc *ResolveDispute) Run(ctx context.Context, cmd ResolveDisputeCommand) (domain.Dispute, error) {
	if !isOperator(cmd.Role) {
		return domain.Dispute{}, domain.ErrNotOperator
	}
	id, err := uuid.Parse(cmd.DisputeID)
	if err != nil {
		return domain.Dispute{}, domain.ErrDisputeNotFound
	}
	decision := domain.DisputeStatus(cmd.Decision)
	if decision != domain.DisputeInFavor && decision != domain.DisputeAgainst {
		return domain.Dispute{}, domain.ErrUnknownDisputeDecision
	}
	if _, err := domain.ReviewDecisionReason(cmd.Reason); err != nil {
		return domain.Dispute{}, err
	}
	return uc.disputes.ResolveDispute(ctx, id.String(), func(current domain.Dispute) (domain.Dispute, error) {
		return current.Resolve(decision, cmd.OperatorID, cmd.Reason, uc.clock())
	})
}

// ListOpenDisputes shows the staff the disputes waiting for a decision.
type ListOpenDisputes struct{ disputes ports.DisputeRepository }

func NewListOpenDisputes(disputes ports.DisputeRepository) *ListOpenDisputes {
	return &ListOpenDisputes{disputes: disputes}
}

func (uc *ListOpenDisputes) Run(ctx context.Context, role string) ([]domain.Dispute, error) {
	if !isOperator(role) {
		return nil, domain.ErrNotOperator
	}
	return uc.disputes.ListOpenDisputes(ctx)
}
