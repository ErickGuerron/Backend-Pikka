package order

import (
	"context"
	"log/slog"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Reincidence marks a customer as recidivist when the incidents that count reach the threshold
// inside one window (ADR 0006, section 1). The mark is sticky: it keeps the date of the first time.
type Reincidence struct {
	surcharges ports.SurchargeRepository
	clock      func() time.Time
}

func NewReincidence(surcharges ports.SurchargeRepository) *Reincidence {
	return &Reincidence{surcharges: surcharges, clock: time.Now}
}

// Evaluate returns true only when this call marked the customer.
func (r *Reincidence) Evaluate(ctx context.Context, customerID string) (bool, error) {
	times, err := r.surcharges.CountedIncidentTimes(ctx, customerID)
	if err != nil {
		return false, err
	}
	if !domain.ReincidenceReached(times) {
		return false, nil
	}
	return r.surcharges.MarkCustomerRecidivist(ctx, customerID, r.clock())
}

// evaluateNonFatal runs after the incident is already stored: a failure is logged and the next
// incident evaluates again, so the mark is only delayed.
func (r *Reincidence) evaluateNonFatal(ctx context.Context, customerID string) {
	if r == nil {
		return
	}
	marked, err := r.Evaluate(ctx, customerID)
	if err != nil {
		slog.Default().Error("reincidence not evaluated; the next incident retries", "customer", customerID, "error", err)
		return
	}
	if marked {
		slog.Default().Warn("customer marked as recidivist", "customer", customerID)
	}
}
