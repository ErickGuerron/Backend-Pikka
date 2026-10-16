// Package osrm adapts the shared libs/osrm estimator to the routing domain.
//
// The HTTP client and OSRM response handling live in libs/osrm. This package only converts the
// domain types and the shared errors into the routing domain's own.
package osrm

import (
	"context"
	"errors"
	"time"

	sharedosrm "github.com/ErickGuerron/Backend-Pikka/libs/osrm"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// Estimator estimates road distance and duration for routing, backed by the shared OSRM module.
type Estimator struct {
	inner *sharedosrm.Estimator
}

// NewEstimator targets an OSRM server such as http://osrm:5000 with a per-call timeout.
func NewEstimator(baseURL string, timeout time.Duration) *Estimator {
	return &Estimator{inner: sharedosrm.NewEstimator(baseURL, timeout)}
}

// Estimate returns the driving distance and duration between two domain points.
func (e *Estimator) Estimate(ctx context.Context, from, to domain.Point) (domain.Estimate, error) {
	got, err := e.inner.Estimate(ctx,
		sharedosrm.Point{Latitude: from.Latitude, Longitude: from.Longitude},
		sharedosrm.Point{Latitude: to.Latitude, Longitude: to.Longitude},
	)
	if err != nil {
		return domain.Estimate{}, toDomainError(err)
	}
	return domain.Estimate{DistanceMeters: got.DistanceMeters, DurationSeconds: got.DurationSeconds}, nil
}

// toDomainError maps the shared sentinel errors to the routing domain's. Other errors pass through.
func toDomainError(err error) error {
	switch {
	case errors.Is(err, sharedosrm.ErrInvalidPoint):
		return domain.ErrInvalidPoint
	case errors.Is(err, sharedosrm.ErrNoRoute):
		return domain.ErrNoRoute
	default:
		return err
	}
}
