// Package position records driver positions and estimates arrival from the latest fix.
package position

import (
	"context"
	"errors"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

var (
	// ErrNoPosition means the driver has no fix in the store (never reported, or expired).
	ErrNoPosition = errors.New("no position known for the driver")
	// ErrStalePosition means the latest fix is older than the fresh window: no estimate is made.
	ErrStalePosition = errors.New("latest driver position is stale")
)

// PositionStore keeps the latest fix per driver. Implementations expire entries after ttl.
type PositionStore interface {
	Save(ctx context.Context, p domain.Position, ttl time.Duration) error
	Latest(ctx context.Context, driverID string) (domain.Position, bool, error)
}

// RouteEstimator returns the road distance and duration between two points.
type RouteEstimator interface {
	Estimate(ctx context.Context, from, to domain.Point) (domain.Estimate, error)
}

// Tracker is the position use case. now is the server clock.
type Tracker struct {
	store     PositionStore
	estimator RouteEstimator
	now       func() time.Time
}

// NewTracker wires the use case to its ports.
func NewTracker(store PositionStore, estimator RouteEstimator, now func() time.Time) *Tracker {
	return &Tracker{store: store, estimator: estimator, now: now}
}

// RecordPosition validates a fix and stores it when it is newer than the stored one. It reports
// whether the fix was accepted; an out-of-order fix is discarded without error.
func (t *Tracker) RecordPosition(ctx context.Context, driverID string, latitude, longitude float64, recordedAt time.Time) (bool, error) {
	fix, err := domain.NewPosition(driverID, latitude, longitude, recordedAt, t.now())
	if err != nil {
		return false, err
	}
	prev, ok, err := t.store.Latest(ctx, driverID)
	if err != nil {
		return false, err
	}
	if ok && !fix.Supersedes(prev) {
		return false, nil
	}
	if err := t.store.Save(ctx, fix, domain.FreshWindow); err != nil {
		return false, err
	}
	return true, nil
}

// EstimateArrival estimates the road distance and time from the driver's latest fresh position
// to dest. A missing or stale position is an error, never a guessed estimate.
func (t *Tracker) EstimateArrival(ctx context.Context, driverID string, dest domain.Point) (domain.Estimate, error) {
	last, ok, err := t.store.Latest(ctx, driverID)
	if err != nil {
		return domain.Estimate{}, err
	}
	if !ok {
		return domain.Estimate{}, ErrNoPosition
	}
	now := t.now()
	if !last.IsFresh(now) {
		return domain.Estimate{}, ErrStalePosition
	}
	from := domain.Point{Latitude: last.Latitude, Longitude: last.Longitude}
	return t.estimator.Estimate(ctx, from, dest)
}
