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
	// ClearRecidivistMark runs in one transaction: it locks the customer, calls build with the mark, stores
	// the audit row and sets recidivist_at to NULL. An unknown customer returns ErrCustomerNotFound and one
	// without the mark ErrCustomerNotRecidivist. Charges already collected and the balance are not touched.
	ClearRecidivistMark(ctx context.Context, customerID string, build func(recidivistAt time.Time) (domain.RecidivistClear, error)) (domain.RecidivistClear, error)
	// ListRecidivists returns the customers with the reincidence mark, oldest mark first.
	ListRecidivists(ctx context.Context) ([]domain.Recidivist, error)
}
