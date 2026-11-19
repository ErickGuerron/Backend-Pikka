package osrm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

var (
	quito      = domain.Point{Latitude: -0.18, Longitude: -78.48}
	quitoSouth = domain.Point{Latitude: -0.22, Longitude: -78.50}
)

// newServer answers every request with the given status and body.
func newServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The routing domain receives the shared module's estimate unchanged.
func TestMapsEstimateToDomain(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"code":"Ok","routes":[{"distance":6283.2,"duration":652.9}]}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	got, err := est.Estimate(context.Background(), quito, quitoSouth)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if got.DistanceMeters != 6283.2 || got.DurationSeconds != 652.9 {
		t.Fatalf("estimate = %+v", got)
	}
}

// "No route" from the shared module must reach the domain as domain.ErrNoRoute.
func TestNoSegmentMapsToDomainNoRoute(t *testing.T) {
	srv := newServer(t, http.StatusBadRequest, `{"code":"NoSegment","message":"Could not find a matching segment"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	_, err := est.Estimate(context.Background(), quito, quitoSouth)
	if !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}

// Invalid coordinates must reach the domain as domain.ErrInvalidPoint.
func TestInvalidPointMapsToDomainInvalidPoint(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"code":"Ok"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	bad := domain.Point{Latitude: 91, Longitude: 0}
	if _, err := est.Estimate(context.Background(), bad, quitoSouth); !errors.Is(err, domain.ErrInvalidPoint) {
		t.Fatalf("err = %v, want domain.ErrInvalidPoint", err)
	}
}
