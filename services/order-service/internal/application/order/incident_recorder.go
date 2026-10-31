package order

import (
	"context"
	"log/slog"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// IncidentRecorder is the one place that turns a penalised not-delivery into a surcharge incident: it prices
// it at 15 % of the undelivered order, stores it once (order_id is unique) and, for a new incident that counts
// now, evaluates reincidence. Closing a stop and the reconciliation sweep both use it, so a recovered incident
// is exactly the one the live path would have recorded.
type IncidentRecorder struct {
	surcharges  ports.SurchargeRepository
	reincidence *Reincidence
}

func NewIncidentRecorder(surcharges ports.SurchargeRepository, reincidence *Reincidence) *IncidentRecorder {
	return &IncidentRecorder{surcharges: surcharges, reincidence: reincidence}
}

// Record stores the incident dated at `at` and reports whether it was created now. A repeat returns the stored
// one with created=false and changes nothing.
func (r *IncidentRecorder) Record(ctx context.Context, customerID, orderID, reason string, at time.Time) (bool, error) {
	incident, err := domain.NewSurchargeIncident(customerID, orderID, reason, at)
	if err != nil {
		return false, err
	}
	saved, created, err := r.surcharges.RecordSurchargeIncident(ctx, r.price(ctx, incident))
	if err != nil {
		return false, err
	}
	// Only a new incident that counts now can complete the threshold; one waiting for the operator counts later.
	if created && saved.Status == domain.SurchargePending {
		r.reincidence.evaluateNonFatal(ctx, saved.CustomerID)
	}
	return created, nil
}

// price sets the 15 % over the undelivered order's base price. An order without lines, a failed lookup or a base
// too small to price leave the incident unpriced: it is still recorded, so the penalty is never lost, and the
// cause is logged.
func (r *IncidentRecorder) price(ctx context.Context, incident domain.Surcharge) domain.Surcharge {
	base, found, err := r.surcharges.OrderBasePrice(ctx, incident.OrderID)
	if err != nil {
		slog.Default().Error("surcharge base price unavailable; recorded unpriced", "order", incident.OrderID, "error", err)
		return incident
	}
	if !found {
		slog.Default().Warn("order has no lines; surcharge recorded unpriced", "order", incident.OrderID)
		return incident
	}
	priced, err := incident.Price(base)
	if err != nil {
		slog.Default().Error("surcharge could not be priced; recorded unpriced", "order", incident.OrderID, "error", err)
		return incident
	}
	return priced
}
