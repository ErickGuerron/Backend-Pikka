package osrm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var (
	tripOrigin = Point{Latitude: -0.18, Longitude: -78.48}
	tripStopA  = Point{Latitude: -0.20, Longitude: -78.50}
	tripStopB  = Point{Latitude: -0.22, Longitude: -78.46}
	tripStopC  = Point{Latitude: -0.19, Longitude: -78.47}
)

// tripServer answers /trip requests with the given body and status, counting calls.
func tripServer(t *testing.T, status int, body string, calls *atomic.Int32, lastURL *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		*lastURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOptimizeReturnsVisitingOrderFromOSRM(t *testing.T) {
	// Input order is A, B, C. OSRM visits B first (waypoint 1), then A (2), then C (3).
	body := `{"code":"Ok",
		"waypoints":[
			{"waypoint_index":0},
			{"waypoint_index":2},
			{"waypoint_index":1},
			{"waypoint_index":3}],
		"trips":[{"distance":4200.5,"duration":610.25,
			"geometry":{"type":"LineString","coordinates":[[-78.48,-0.18],[-78.46,-0.22],[-78.5,-0.2],[-78.47,-0.19]]}}]}`
	var calls atomic.Int32
	var last string
	srv := tripServer(t, http.StatusOK, body, &calls, &last)
	tripper := NewTripper(srv.URL, time.Second)

	got, err := tripper.Optimize(context.Background(), tripOrigin, []Point{tripStopA, tripStopB, tripStopC})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if want := []int{1, 0, 2}; !equalInts(got.Order, want) {
		t.Fatalf("Order = %v, want %v (indexes into the input stops)", got.Order, want)
	}
	if got.DistanceMeters != 4200.5 || got.DurationSeconds != 610.25 {
		t.Fatalf("totals = %v m / %v s", got.DistanceMeters, got.DurationSeconds)
	}
	// GeoJSON is lon,lat; the result is lat,lon like every other point in this module.
	wantGeometry := []Point{
		{Latitude: -0.18, Longitude: -78.48},
		{Latitude: -0.22, Longitude: -78.46},
		{Latitude: -0.2, Longitude: -78.5},
		{Latitude: -0.19, Longitude: -78.47},
	}
	if len(got.Geometry) != len(wantGeometry) {
		t.Fatalf("geometry has %d points, want %d", len(got.Geometry), len(wantGeometry))
	}
	for i := range wantGeometry {
		if got.Geometry[i] != wantGeometry[i] {
			t.Fatalf("geometry[%d] = %+v, want %+v", i, got.Geometry[i], wantGeometry[i])
		}
	}
}

func TestOptimizeFixesTheOriginAsFirstStopAndDoesNotReturnToIt(t *testing.T) {
	body := `{"code":"Ok","waypoints":[{"waypoint_index":0},{"waypoint_index":1}],
		"trips":[{"distance":1,"duration":1,"geometry":{"coordinates":[[-78.48,-0.18],[-78.5,-0.2]]}}]}`
	var calls atomic.Int32
	var last string
	srv := tripServer(t, http.StatusOK, body, &calls, &last)

	if _, err := NewTripper(srv.URL, time.Second).Optimize(context.Background(), tripOrigin, []Point{tripStopA}); err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if !strings.Contains(last, "/trip/v1/driving/-78.48,-0.18;-78.5,-0.2?") {
		t.Fatalf("request path = %q, want origin first then the stop, lon,lat", last)
	}
	if !strings.Contains(last, "source=first") || !strings.Contains(last, "roundtrip=false") {
		t.Fatalf("request query = %q, want source=first and roundtrip=false", last)
	}
	if !strings.Contains(last, "geometries=geojson") || !strings.Contains(last, "overview=full") {
		t.Fatalf("request query = %q, want geometry for the drawn route", last)
	}
}

func TestOptimizeWithoutStopsMakesNoCall(t *testing.T) {
	var calls atomic.Int32
	var last string
	srv := tripServer(t, http.StatusOK, `{"code":"Ok"}`, &calls, &last)

	got, err := NewTripper(srv.URL, time.Second).Optimize(context.Background(), tripOrigin, nil)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if len(got.Order) != 0 || len(got.Geometry) != 0 {
		t.Fatalf("empty tour = %+v, want no order and no geometry", got)
	}
	if calls.Load() != 0 {
		t.Fatalf("OSRM was called %d times for an empty tour", calls.Load())
	}
}

func TestOptimizeRejectsInvalidPointsBeforeAnyCall(t *testing.T) {
	var calls atomic.Int32
	var last string
	srv := tripServer(t, http.StatusOK, `{"code":"Ok"}`, &calls, &last)
	tripper := NewTripper(srv.URL, time.Second)

	bad := Point{Latitude: 91, Longitude: -78.5}
	if _, err := tripper.Optimize(context.Background(), tripOrigin, []Point{tripStopA, bad}); !errors.Is(err, ErrInvalidPoint) {
		t.Fatalf("invalid stop: err = %v, want ErrInvalidPoint", err)
	}
	if _, err := tripper.Optimize(context.Background(), bad, []Point{tripStopA}); !errors.Is(err, ErrInvalidPoint) {
		t.Fatalf("invalid origin: err = %v, want ErrInvalidPoint", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("OSRM was called %d times for invalid input", calls.Load())
	}
}

func TestOptimizeMapsNoSegmentToErrNoRoute(t *testing.T) {
	// OSRM answers a point off the map with HTTP 400 and code NoSegment.
	var calls atomic.Int32
	var last string
	srv := tripServer(t, http.StatusBadRequest, `{"code":"NoSegment","message":"Could not find a matching segment"}`, &calls, &last)

	if _, err := NewTripper(srv.URL, time.Second).Optimize(context.Background(), tripOrigin, []Point{tripStopA}); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

func TestOptimizeFailsOnAnUnknownOSRMCode(t *testing.T) {
	var calls atomic.Int32
	var last string
	srv := tripServer(t, http.StatusOK, `{"code":"InvalidQuery"}`, &calls, &last)

	_, err := NewTripper(srv.URL, time.Second).Optimize(context.Background(), tripOrigin, []Point{tripStopA})
	if err == nil || errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want a plain error that is not ErrNoRoute", err)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
