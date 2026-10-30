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
	// ResolveSurchargeVerification locks the surcharge, lets decide change it and stores the result,
	// so two operators cannot resolve the same verification. Unknown id returns ErrSurchargeNotFound.
	ResolveSurchargeVerification(ctx context.Context, id string, decide func(domain.Surcharge) (domain.Surcharge, error)) (domain.Surcharge, error)
	// ListSurchargesPendingVerification is the operator's queue, oldest first.
	ListSurchargesPendingVerification(ctx context.Context) ([]domain.Surcharge, error)
}
