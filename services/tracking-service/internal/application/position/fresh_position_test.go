package position

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

func TestFreshPositionReturnsTheLatestFreshFix(t *testing.T) {
	store := newFakeStore()
	recorded := clock.Add(-30 * time.Second)
	store.latest["driver-1"] = domain.Position{DriverID: "driver-1", Latitude: -0.18, Longitude: -78.48, RecordedAt: recorded}
	tr := newTracker(store, &fakeEstimator{})

	got, err := tr.FreshPosition(context.Background(), "driver-1")

	if err != nil {
		t.Fatalf("FreshPosition: %v", err)
	}
	if got.Latitude != -0.18 || got.Longitude != -78.48 || !got.RecordedAt.Equal(recorded) {
		t.Fatalf("position = %+v, want the stored fix", got)
	}
}

func TestFreshPositionStaleFixIsErrStalePosition(t *testing.T) {
	store := newFakeStore()
	store.latest["driver-1"] = domain.Position{DriverID: "driver-1", Latitude: -0.18, Longitude: -78.48, RecordedAt: clock.Add(-3 * time.Minute)}
	tr := newTracker(store, &fakeEstimator{})

	_, err := tr.FreshPosition(context.Background(), "driver-1")

	if !errors.Is(err, ErrStalePosition) {
		t.Fatalf("err = %v, want ErrStalePosition", err)
	}
}

func TestFreshPositionUnknownDriverIsErrNoPosition(t *testing.T) {
	tr := newTracker(newFakeStore(), &fakeEstimator{})

	_, err := tr.FreshPosition(context.Background(), "nobody")

	if !errors.Is(err, ErrNoPosition) {
		t.Fatalf("err = %v, want ErrNoPosition", err)
	}
}
