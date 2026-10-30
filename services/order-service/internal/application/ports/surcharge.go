package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// SurchargeRepository stores penalty incidences.
type SurchargeRepository interface {
	// RecordSurchargeIncident saves the incidence once per order. If the order already has one it
	// returns the stored row and created=false, so retries and concurrent events never charge twice.
	RecordSurchargeIncident(ctx context.Context, s domain.Surcharge) (saved domain.Surcharge, created bool, err error)
}
