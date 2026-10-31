package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ReassignmentRepository stores the fixed reassignment fee and the charges made with it.
type ReassignmentRepository interface {
	// SetReassignmentFee stores a new fee; the latest one is the current fee.
	SetReassignmentFee(ctx context.Context, fee domain.ReassignmentFee) (domain.ReassignmentFee, error)
	// CurrentReassignmentFee returns the latest fee, or ErrReassignmentFeeNotSet when none was ever set.
	CurrentReassignmentFee(ctx context.Context) (domain.ReassignmentFee, error)
	// ApplyReassignmentCharge runs in one transaction: it locks the order's customer, reads the current fee (nil
	// when none) and calls build with the customer id, stores the charge and adds its amount to
	// customers.pending_surcharge. An unknown order returns ErrOrderNotFound and a second charge for the same
	// order ErrReassignmentChargeExists. Nothing is stored on error.
	ApplyReassignmentCharge(ctx context.Context, orderID string, build func(customerID string, fee *domain.ReassignmentFee) (domain.ReassignmentCharge, error)) (domain.ReassignmentCharge, error)
}
