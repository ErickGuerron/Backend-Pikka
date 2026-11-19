package osrm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Trip is the best visiting order of a set of stops, starting at an origin, and the road it follows.
type Trip struct {
	// Order holds indexes into the stops passed to Optimize, in the order they should be visited.
	// The origin is not part of it, and the tour does not return to the origin.
	Order           []int
	DistanceMeters  float64
	DurationSeconds float64
	// Geometry is the road the tour follows, origin first, one point per road vertex.
	Geometry []Point
}

// Tripper asks OSRM's trip service (the traveling-salesman solver) for the best order of a set of stops.
type Tripper struct {
	baseURL string
	client  *http.Client
}

// NewTripper targets an OSRM server such as http://osrm:5000 with a per-call timeout.
func NewTripper(baseURL string, timeout time.Duration) *Tripper {
	return &Tripper{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

type tripResponse struct {
	Code      string `json:"code"`
	Waypoints []struct {
		WaypointIndex int `json:"waypoint_index"`
	} `json:"waypoints"`
	Trips []struct {
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
		Geometry struct {
			Coordinates [][2]float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"trips"`
}

// Optimize returns the visiting order of the stops that minimizes the driving time from the origin, with
// the road geometry. The origin is fixed as the first point and the tour is open (it does not come back).
// No stops means an empty tour with no call. Invalid coordinates fail before any network call; a point
// OSRM cannot snap to a road is ErrNoRoute.
func (t *Tripper) Optimize(ctx context.Context, origin Point, stops []Point) (Trip, error) {
	if len(stops) == 0 {
		return Trip{}, nil
	}
	if !origin.Valid() {
		return Trip{}, ErrInvalidPoint
	}
	coords := []string{lonLat(origin)}
	for _, stop := range stops {
		if !stop.Valid() {
			return Trip{}, ErrInvalidPoint
		}
		coords = append(coords, lonLat(stop))
	}

	// Literal ';' separators, like the route service: url.Values would escape them.
	radiuses := make([]string, len(coords))
	for i := range radiuses {
		radiuses[i] = snapRadiusMeters
	}
	endpoint := t.baseURL + "/trip/v1/driving/" + strings.Join(coords, ";") +
		"?source=first&roundtrip=false&overview=full&geometries=geojson&radiuses=" + strings.Join(radiuses, ";")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Trip{}, fmt.Errorf("osrm trip request: %w", err)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return Trip{}, fmt.Errorf("osrm trip call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes*4))
	if err != nil {
		return Trip{}, fmt.Errorf("osrm trip read: %w", err)
	}

	var parsed tripResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode != http.StatusOK {
			return Trip{}, fmt.Errorf("osrm trip status %d", resp.StatusCode)
		}
		return Trip{}, fmt.Errorf("osrm trip decode: %w", err)
	}
	// Same rule as the route service: the body code decides before the HTTP status.
	if parsed.Code == "NoRoute" || parsed.Code == "NoSegment" || parsed.Code == "NoTrips" {
		return Trip{}, ErrNoRoute
	}
	if resp.StatusCode != http.StatusOK {
		return Trip{}, fmt.Errorf("osrm trip status %d", resp.StatusCode)
	}
	if parsed.Code != "Ok" {
		return Trip{}, fmt.Errorf("osrm trip code %q", parsed.Code)
	}
	if len(parsed.Trips) == 0 || len(parsed.Waypoints) != len(coords) {
		return Trip{}, ErrNoRoute
	}

	// waypoints[i] describes input point i; its waypoint_index is its place in the tour. The origin is
	// input 0 and stays first; the stops are the rest, sorted by their place in the tour.
	order := make([]int, 0, len(stops))
	for i := 1; i < len(parsed.Waypoints); i++ {
		order = append(order, i-1)
	}
	sort.SliceStable(order, func(a, b int) bool {
		return parsed.Waypoints[order[a]+1].WaypointIndex < parsed.Waypoints[order[b]+1].WaypointIndex
	})

	trip := parsed.Trips[0]
	geometry := make([]Point, 0, len(trip.Geometry.Coordinates))
	for _, c := range trip.Geometry.Coordinates {
		// GeoJSON is [lon, lat].
		geometry = append(geometry, Point{Latitude: c[1], Longitude: c[0]})
	}
	return Trip{
		Order:           order,
		DistanceMeters:  trip.Distance,
		DurationSeconds: trip.Duration,
		Geometry:        geometry,
	}, nil
}
