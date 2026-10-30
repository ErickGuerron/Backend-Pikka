package ports

import (
	"context"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RecidivismRepository stores the extra charge an operator applies to a recidivist.
type RecidivismRepository interface {
	// ApplyRecidivismCharge runs in one transaction: it locks the customer, reads the reincidence mark
	// and calls build with it, stores the charge and adds its amount to customers.pending_surcharge.
	// An unknown customer returns ErrCustomerNotFound, one without the mark ErrCustomerNotRecidivist,
	// and a second charge for the same mark ErrRecidivismChargeExists. Nothing is stored on error.
	ApplyRecidivismCharge(ctx context.Context, customerID string, build func(recidivistAt time.Time) (domain.RecidivismCharge, error)) (domain.RecidivismCharge, error)
	// ListRecidivists returns the customers with the reincidence mark, oldest mark first.
	ListRecidivists(ctx context.Context) ([]domain.Recidivist, error)
}
