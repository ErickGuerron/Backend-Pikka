package osrm

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

var (
	quito      = domain.Point{Latitude: -0.18, Longitude: -78.48}
	quitoSouth = domain.Point{Latitude: -0.22, Longitude: -78.50}
)

// newServer answers every request with the given status and body and records the last URL it saw.
func newServer(t *testing.T, status int, body string) (*httptest.Server, *string) {
	t.Helper()
	var last string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.RequestURI()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

// OSRM takes coordinates as longitude,latitude and returns distance (m) and duration (s).
func TestEstimateSendsLonLatAndReturnsDistanceAndDuration(t *testing.T) {
	srv, last := newServer(t, http.StatusOK, `{"code":"Ok","routes":[{"distance":6283.2,"duration":652.9}]}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	got, err := est.Estimate(context.Background(), quito, quitoSouth)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if got.DistanceMeters != 6283.2 || got.DurationSeconds != 652.9 {
		t.Fatalf("estimate = %+v", got)
	}
	if !strings.HasPrefix(*last, "/route/v1/driving/-78.48,-0.18;-78.5,-0.22") {
		t.Fatalf("request path = %q, want lon,lat order for both points", *last)
	}
	if !strings.Contains(*last, "overview=false") {
		t.Fatalf("request should not ask for geometry: %q", *last)
	}
	// Without a snapping radius OSRM moves any point, however far, onto the nearest road.
	if !strings.Contains(*last, "radiuses="+snapRadiusMeters+";"+snapRadiusMeters) {
		t.Fatalf("request must limit snapping distance with radiuses: %q", *last)
	}
}

func TestEstimateNoRouteIsDomainError(t *testing.T) {
	srv, _ := newServer(t, http.StatusOK, `{"code":"NoRoute","message":"Impossible route"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	_, err := est.Estimate(context.Background(), quito, quitoSouth)
	if !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}

// Invalid coordinates are rejected before any HTTP call.
func TestEstimateRejectsInvalidPointsWithoutCalling(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	est := NewEstimator(srv.URL, 2*time.Second)

	bad := []domain.Point{
		{Latitude: 91, Longitude: 0},
		{Latitude: 0, Longitude: -181},
		{Latitude: math.NaN(), Longitude: 0},
		{Latitude: 0, Longitude: math.Inf(1)},
	}
	for _, p := range bad {
		if _, err := est.Estimate(context.Background(), p, quitoSouth); !errors.Is(err, domain.ErrInvalidPoint) {
			t.Fatalf("point %+v: err = %v, want ErrInvalidPoint", p, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid points reached the server %d times", calls.Load())
	}
}

func TestEstimateReportsNonOKStatus(t *testing.T) {
	srv, _ := newServer(t, http.StatusInternalServerError, `oops`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), quito, quitoSouth); err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestEstimateReportsMalformedBody(t *testing.T) {
	srv, _ := newServer(t, http.StatusOK, `{"code":`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), quito, quitoSouth); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestEstimateOkWithoutRoutesIsNoRoute(t *testing.T) {
	srv, _ := newServer(t, http.StatusOK, `{"code":"Ok","routes":[]}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), quito, quitoSouth); !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}

// A slow OSRM must not block tracking: the timeout cuts the call.
func TestEstimateHonoursTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	est := NewEstimator(srv.URL, 100*time.Millisecond)

	start := time.Now()
	if _, err := est.Estimate(context.Background(), quito, quitoSouth); err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout not honoured, took %s", elapsed)
	}
}

// A point outside the map or far from any road is "NoSegment" in OSRM: no route, not a server failure.
func TestEstimateNoSegmentIsDomainError(t *testing.T) {
	srv, _ := newServer(t, http.StatusOK, `{"code":"NoSegment","message":"Could not find a matching segment"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), quito, quitoSouth); !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}

// OSRM answers an unreachable point with HTTP 400 and a NoSegment body, not with 200.
func TestEstimateNoSegmentWithHTTP400IsDomainError(t *testing.T) {
	srv, _ := newServer(t, http.StatusBadRequest, `{"code":"NoSegment","message":"Could not find a matching segment"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	if _, err := est.Estimate(context.Background(), quito, quitoSouth); !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("err = %v, want domain.ErrNoRoute", err)
	}
}
