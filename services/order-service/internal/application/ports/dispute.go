package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// DisputeRepository stores the customers' disputes over surcharges (ADR 0007, section 6).
type DisputeRepository interface {
	// OpenDispute locks the surcharge and calls build with it. A surcharge that does not exist, or that belongs to
	// another customer, returns ErrSurchargeNotFound (the same answer for both: nobody learns which ids exist). A second
	// dispute over the same surcharge returns ErrDisputeExists. Nothing is stored on error.
	OpenDispute(ctx context.Context, surchargeID, customerID string, build func(domain.Surcharge) (domain.Dispute, error)) (domain.Dispute, error)
	// ResolveDispute locks the dispute and lets decide change it. An unknown id returns ErrDisputeNotFound. When the
	// decision is IN_FAVOR the surcharge is waived and taken off the customer's balance in the same transaction; if it
	// is no longer owed (it was paid meanwhile) it returns ErrSurchargeNotDisputable and the dispute stays open.
	ResolveDispute(ctx context.Context, disputeID string, decide func(domain.Dispute) (domain.Dispute, error)) (domain.Dispute, error)
	// ListOpenDisputes is the staff's queue, oldest first.
	ListOpenDisputes(ctx context.Context) ([]domain.Dispute, error)
}
