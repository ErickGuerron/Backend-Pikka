package position

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

var clock = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type fakeStore struct {
	latest    map[string]domain.Position
	saved     []domain.Position
	savedTTLs []time.Duration
	err       error
}

func newFakeStore() *fakeStore { return &fakeStore{latest: map[string]domain.Position{}} }

func (f *fakeStore) Save(_ context.Context, p domain.Position, ttl time.Duration) error {
	if f.err != nil {
		return f.err
	}
	f.latest[p.DriverID] = p
	f.saved = append(f.saved, p)
	f.savedTTLs = append(f.savedTTLs, ttl)
	return nil
}

func (f *fakeStore) Latest(_ context.Context, driverID string) (domain.Position, bool, error) {
	p, ok := f.latest[driverID]
	return p, ok, nil
}

type fakeEstimator struct {
	calls []struct{ from, to domain.Point }
	out   domain.Estimate
	err   error
}

func (f *fakeEstimator) Estimate(_ context.Context, from, to domain.Point) (domain.Estimate, error) {
	f.calls = append(f.calls, struct{ from, to domain.Point }{from, to})
	return f.out, f.err
}

func newTracker(store *fakeStore, est *fakeEstimator) *Tracker {
	return NewTracker(store, est, func() time.Time { return clock })
}

func TestRecordPositionStoresFirstFixWithFreshWindowTTL(t *testing.T) {
	store := newFakeStore()
	tr := newTracker(store, &fakeEstimator{})

	accepted, err := tr.RecordPosition(context.Background(), "driver-1", -0.18, -78.48, clock)
	if err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v, want accepted", accepted, err)
	}
	if len(store.saved) != 1 || store.savedTTLs[0] != domain.FreshWindow {
		t.Fatalf("saved=%d ttl=%v, want one save with the fresh window", len(store.saved), store.savedTTLs)
	}
}

func TestRecordPositionDiscardsOutOfOrderFix(t *testing.T) {
	store := newFakeStore()
	tr := newTracker(store, &fakeEstimator{})
	ctx := context.Background()
	if _, err := tr.RecordPosition(ctx, "driver-1", 1, 1, clock); err != nil {
		t.Fatal(err)
	}

	accepted, err := tr.RecordPosition(ctx, "driver-1", 2, 2, clock.Add(-10*time.Second))
	if err != nil {
		t.Fatalf("err = %v, want nil for a discarded fix", err)
	}
	if accepted || len(store.saved) != 1 {
		t.Fatalf("out-of-order fix must be discarded: accepted=%v saves=%d", accepted, len(store.saved))
	}
}

func TestRecordPositionRejectsInvalidFixWithoutStoring(t *testing.T) {
	store := newFakeStore()
	tr := newTracker(store, &fakeEstimator{})

	_, err := tr.RecordPosition(context.Background(), "driver-1", 91, 0, clock)
	if !errors.Is(err, domain.ErrInvalidPosition) {
		t.Fatalf("err = %v, want ErrInvalidPosition", err)
	}
	if len(store.saved) != 0 {
		t.Fatal("invalid fix must not reach the store")
	}
}

func TestEstimateArrivalWithoutPositionIsNoPosition(t *testing.T) {
	est := &fakeEstimator{}
	tr := newTracker(newFakeStore(), est)

	_, err := tr.EstimateArrival(context.Background(), "driver-1", domain.Point{Latitude: -0.2, Longitude: -78.5})
	if !errors.Is(err, ErrNoPosition) {
		t.Fatalf("err = %v, want ErrNoPosition", err)
	}
	if len(est.calls) != 0 {
		t.Fatal("no estimate may be requested without a position")
	}
}

// A stale position must never produce an estimate: the driver may have moved far since.
func TestEstimateArrivalWithStalePositionIsNotEstimated(t *testing.T) {
	store := newFakeStore()
	stale, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock.Add(-121*time.Second), clock)
	store.latest["driver-1"] = stale
	est := &fakeEstimator{}
	tr := newTracker(store, est)

	_, err := tr.EstimateArrival(context.Background(), "driver-1", domain.Point{Latitude: -0.2, Longitude: -78.5})
	if !errors.Is(err, ErrStalePosition) {
		t.Fatalf("err = %v, want ErrStalePosition", err)
	}
	if len(est.calls) != 0 {
		t.Fatal("stale position must not reach the estimator")
	}
}

func TestEstimateArrivalUsesFreshPositionAndDestination(t *testing.T) {
	store := newFakeStore()
	fresh, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock.Add(-5*time.Second), clock)
	store.latest["driver-1"] = fresh
	est := &fakeEstimator{out: domain.Estimate{DistanceMeters: 6283, DurationSeconds: 653}}
	tr := newTracker(store, est)
	dest := domain.Point{Latitude: -0.22, Longitude: -78.50}

	got, err := tr.EstimateArrival(context.Background(), "driver-1", dest)
	if err != nil {
		t.Fatalf("EstimateArrival: %v", err)
	}
	if got.DistanceMeters != 6283 || got.DurationSeconds != 653 {
		t.Fatalf("estimate = %+v", got)
	}
	if len(est.calls) != 1 || est.calls[0].from != (domain.Point{Latitude: -0.18, Longitude: -78.48}) || est.calls[0].to != dest {
		t.Fatalf("estimator calls = %+v", est.calls)
	}
}

func TestEstimateArrivalPropagatesNoRoute(t *testing.T) {
	store := newFakeStore()
	fresh, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock, clock)
	store.latest["driver-1"] = fresh
	tr := newTracker(store, &fakeEstimator{err: domain.ErrNoRoute})

	_, err := tr.EstimateArrival(context.Background(), "driver-1", domain.Point{Latitude: 4.7, Longitude: -74.0})
	if !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}
