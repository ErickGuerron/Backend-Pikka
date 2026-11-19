package order

import (
	"context"
	"log/slog"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
)

// reconcileBatch bounds one pass. A bigger backlog is drained over the next passes.
const reconcileBatch = 200

// ReconcileIncidents recovers the surcharge incidents that were lost when a stop closed (ADR 0006): the driver
// cannot retry a closed stop, so without this the penalty would be gone and the customer would not count towards
// reincidence. It is safe to run any number of times: an incident is stored once per order.
type ReconcileIncidents struct {
	surcharges ports.SurchargeRepository
	recorder   *IncidentRecorder
}

func NewReconcileIncidents(surcharges ports.SurchargeRepository, recorder *IncidentRecorder) *ReconcileIncidents {
	return &ReconcileIncidents{surcharges: surcharges, recorder: recorder}
}

// Run returns how many incidents it created. One failing incident is logged and does not stop the others; only a
// failure to list the lost ones is returned.
func (uc *ReconcileIncidents) Run(ctx context.Context) (int, error) {
	lost, err := uc.surcharges.ListUnrecordedIncidents(ctx, reconcileBatch)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, incident := range lost {
		created, err := uc.recorder.Record(ctx, incident.CustomerID, incident.OrderID, incident.Reason, incident.RecordedAt)
		if err != nil {
			slog.Default().Error("lost surcharge incident not recovered; the next pass retries", "order", incident.OrderID, "error", err)
			continue
		}
		if created {
			recovered++
		}
	}
	return recovered, nil
}
