package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ResolveSurchargeCommand is the operator's decision about a surcharge waiting for verification
// (ADR 0006 amendment for WRONG_ADDRESS).
type ResolveSurchargeCommand struct {
	Role        string
	OperatorID  string
	SurchargeID string
	Decision    string
	Reason      string
}

// ResolveSurchargeVerification lets an operator or admin confirm or waive a PENDING_VERIFICATION
// surcharge, and list the ones still waiting.
type ResolveSurchargeVerification struct {
	surcharges ports.SurchargeRepository
	clock      func() time.Time
}

func NewResolveSurchargeVerification(surcharges ports.SurchargeRepository) *ResolveSurchargeVerification {
	return &ResolveSurchargeVerification{surcharges: surcharges, clock: time.Now}
}

// Run validates the input and resolves under the repository's lock.
func (uc *ResolveSurchargeVerification) Run(ctx context.Context, cmd ResolveSurchargeCommand) (domain.Surcharge, error) {
	if !isOperator(cmd.Role) {
		return domain.Surcharge{}, domain.ErrNotOperator
	}
	id, err := uuid.Parse(cmd.SurchargeID)
	if err != nil {
		return domain.Surcharge{}, domain.ErrSurchargeNotFound
	}
	decision := domain.SurchargeDecision(cmd.Decision)
	if decision != domain.DecisionConfirm && decision != domain.DecisionWaive {
		return domain.Surcharge{}, domain.ErrUnknownSurchargeDecision
	}
	if _, err := domain.ReviewDecisionReason(cmd.Reason); err != nil {
		return domain.Surcharge{}, err
	}
	return uc.surcharges.ResolveSurchargeVerification(ctx, id.String(), func(current domain.Surcharge) (domain.Surcharge, error) {
		return current.Resolve(decision, cmd.OperatorID, cmd.Reason, uc.clock())
	})
}

// ListPending returns the operator's queue of surcharges waiting for verification.
func (uc *ResolveSurchargeVerification) ListPending(ctx context.Context, role string) ([]domain.Surcharge, error) {
	if !isOperator(role) {
		return nil, domain.ErrNotOperator
	}
	return uc.surcharges.ListSurchargesPendingVerification(ctx)
}
