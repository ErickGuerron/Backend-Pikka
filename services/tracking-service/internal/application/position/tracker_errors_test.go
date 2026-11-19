package position

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

var errStoreDown = errors.New("store down")

// brokenStore falla todas las operaciones con el error configurado.
type brokenStore struct{ err error }

func (b brokenStore) Save(context.Context, domain.Position, time.Duration) error { return b.err }

func (b brokenStore) Latest(context.Context, string) (domain.Position, bool, error) {
	return domain.Position{}, false, b.err
}

func TestRecordPositionRejectsInvalidFixBeforeStoring(t *testing.T) {
	store := newFakeStore()
	tr := newTracker(store, &fakeEstimator{})

	accepted, err := tr.RecordPosition(context.Background(), "driver-1", 95, 0, clock)

	if accepted || !errors.Is(err, domain.ErrInvalidPosition) {
		t.Fatalf("accepted=%v err=%v, want ErrInvalidPosition", accepted, err)
	}
	if len(store.saved) != 0 {
		t.Fatal("un punto inválido no se guarda")
	}
}

func TestRecordPositionReportsStoreFailures(t *testing.T) {
	tr := NewTracker(brokenStore{err: errStoreDown}, &fakeEstimator{}, func() time.Time { return clock })
	if _, err := tr.RecordPosition(context.Background(), "driver-1", 0, 0, clock); !errors.Is(err, errStoreDown) {
		t.Fatalf("read failure: err = %v, want the store error", err)
	}

	store := newFakeStore()
	store.err = errStoreDown
	tr = newTracker(store, &fakeEstimator{})
	accepted, err := tr.RecordPosition(context.Background(), "driver-1", 0, 0, clock)
	if accepted || !errors.Is(err, errStoreDown) {
		t.Fatalf("write failure: accepted=%v err=%v, want the store error", accepted, err)
	}
}

func TestFreshPositionReportsStoreAndMissingFix(t *testing.T) {
	_, err := NewTracker(brokenStore{err: errStoreDown}, &fakeEstimator{}, func() time.Time { return clock }).
		FreshPosition(context.Background(), "driver-1")
	if !errors.Is(err, errStoreDown) {
		t.Fatalf("err = %v, want the store error", err)
	}

	_, err = newTracker(newFakeStore(), &fakeEstimator{}).FreshPosition(context.Background(), "driver-1")
	if !errors.Is(err, ErrNoPosition) {
		t.Fatalf("err = %v, want ErrNoPosition", err)
	}
}

func TestEstimateArrivalReportsStoreFailureAndEstimatorErrors(t *testing.T) {
	_, err := NewTracker(brokenStore{err: errStoreDown}, &fakeEstimator{}, func() time.Time { return clock }).
		EstimateArrival(context.Background(), "driver-1", domain.Point{})
	if !errors.Is(err, errStoreDown) {
		t.Fatalf("store failure: err = %v", err)
	}

	store := newFakeStore()
	_, err = newTracker(store, &fakeEstimator{}).EstimateArrival(context.Background(), "driver-1", domain.Point{})
	if !errors.Is(err, ErrNoPosition) {
		t.Fatalf("no fix: err = %v, want ErrNoPosition", err)
	}

	store.latest["driver-1"] = domain.Position{DriverID: "driver-1", Latitude: -0.18, Longitude: -78.48, RecordedAt: clock.Add(-10 * time.Second)}
	est := &fakeEstimator{err: domain.ErrNoRoute}
	_, err = newTracker(store, est).EstimateArrival(context.Background(), "driver-1", domain.Point{Latitude: -0.2, Longitude: -78.5})
	if !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("estimator failure: err = %v, want ErrNoRoute passed through", err)
	}
	if len(est.calls) != 1 {
		t.Fatalf("estimator calls = %d, want 1", len(est.calls))
	}
}

func TestFreshPositionRejectsStaleFixFromStore(t *testing.T) {
	store := newFakeStore()
	store.latest["driver-1"] = domain.Position{DriverID: "driver-1", RecordedAt: clock.Add(-time.Hour)}

	_, err := newTracker(store, &fakeEstimator{}).FreshPosition(context.Background(), "driver-1")

	if !errors.Is(err, ErrStalePosition) {
		t.Fatalf("err = %v, want ErrStalePosition", err)
	}
}
