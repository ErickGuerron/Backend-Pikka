package osrm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

func newServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

var (
	driver = domain.Point{Latitude: -0.18, Longitude: -78.48}
	dest   = domain.Point{Latitude: -0.22, Longitude: -78.50}
)

func TestMapsRouteToDomainEstimate(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"code":"Ok","routes":[{"distance":6283.2,"duration":652.9}]}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	got, err := est.Estimate(context.Background(), driver, dest)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if got.DistanceMeters != 6283.2 || got.DurationSeconds != 652.9 {
		t.Fatalf("estimate = %+v", got)
	}
}

// OSRM NoSegment (HTTP 400) must reach the tracking domain as domain.ErrNoRoute.
func TestNoSegmentMapsToDomainNoRoute(t *testing.T) {
	srv := newServer(t, http.StatusBadRequest, `{"code":"NoSegment","message":"Could not find a matching segment"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), driver, dest); !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}

func TestInvalidPointIsRejectedBeforeCall(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"code":"Ok"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), domain.Point{Latitude: 91}, dest); err == nil {
		t.Fatal("expected error for an out-of-range point")
	}
}
