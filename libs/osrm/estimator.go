// Package osrm estimates road distance and driving time through an OSRM HTTP server (osrm-routed).
//
// It is a shared, dependency-free module: each service maps its own domain types to these ones,
// so no service imports another service's domain.
//
// The server holds one map region (Ecuador in the MVP). Its base URL is configuration; the
// package knows nothing about regions, so adding a country means adding a server, not code.
package osrm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Errors returned when estimating the road distance between two points.
var (
	ErrInvalidPoint = errors.New("latitude must be in [-90, 90] and longitude in [-180, 180], both finite")
	ErrNoRoute      = errors.New("no road route between the points")
)

// snapRadiusMeters is the largest distance a point may be from a road and still be snapped to it.
// GPS fixes and geocoded addresses are a few tens of meters off; anything beyond this is not on the map.
const snapRadiusMeters = "1000"

// maxBodyBytes caps the response read from OSRM. A route without geometry is a few hundred bytes.
const maxBodyBytes = 64 << 10

// Point is a WGS 84 coordinate.
type Point struct {
	Latitude  float64
	Longitude float64
}

// Valid reports whether the point is a finite WGS 84 coordinate.
func (p Point) Valid() bool {
	return p.Latitude >= -90 && p.Latitude <= 90 &&
		p.Longitude >= -180 && p.Longitude <= 180 &&
		!math.IsNaN(p.Latitude) && !math.IsNaN(p.Longitude) &&
		!math.IsInf(p.Latitude, 0) && !math.IsInf(p.Longitude, 0)
}

// Estimate is the road distance and driving time between two points.
type Estimate struct {
	DistanceMeters  float64
	DurationSeconds float64
}

// Estimator calls OSRM's route service to estimate distance and duration between two points.
type Estimator struct {
	baseURL string
	client  *http.Client
}

// NewEstimator targets an OSRM server such as http://osrm:5000 with a per-call timeout.
func NewEstimator(baseURL string, timeout time.Duration) *Estimator {
	return &Estimator{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

type routeResponse struct {
	Code   string `json:"code"`
	Routes []struct {
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
	} `json:"routes"`
}

// Estimate returns the driving distance and duration from one point to another.
// Invalid coordinates fail before any network call. OSRM "NoRoute" and an empty routes list
// both map to ErrNoRoute.
func (e *Estimator) Estimate(ctx context.Context, from, to Point) (Estimate, error) {
	if !from.Valid() || !to.Valid() {
		return Estimate{}, ErrInvalidPoint
	}

	// OSRM expects longitude,latitude pairs separated by a literal ';'. The query is built from
	// constants and validated numbers only, so it is written as-is (url.Values would escape ';').
	coords := lonLat(from) + ";" + lonLat(to)
	radiuses := snapRadiusMeters + ";" + snapRadiusMeters
	endpoint := e.baseURL + "/route/v1/driving/" + coords + "?overview=false&radiuses=" + radiuses

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Estimate{}, fmt.Errorf("osrm request: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return Estimate{}, fmt.Errorf("osrm call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Estimate{}, fmt.Errorf("osrm read: %w", err)
	}

	var parsed routeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Some failures are not JSON at all; report the status when it is not OK.
		if resp.StatusCode != http.StatusOK {
			return Estimate{}, fmt.Errorf("osrm status %d", resp.StatusCode)
		}
		return Estimate{}, fmt.Errorf("osrm decode: %w", err)
	}
	// NoSegment: the point is off the map or far from any road. OSRM answers it with HTTP 400,
	// so the body code decides before the status does. Both codes mean "no route".
	if parsed.Code == "NoRoute" || parsed.Code == "NoSegment" {
		return Estimate{}, ErrNoRoute
	}
	if resp.StatusCode != http.StatusOK {
		return Estimate{}, fmt.Errorf("osrm status %d", resp.StatusCode)
	}
	if len(parsed.Routes) == 0 {
		return Estimate{}, ErrNoRoute
	}
	if parsed.Code != "Ok" {
		return Estimate{}, fmt.Errorf("osrm code %q", parsed.Code)
	}
	route := parsed.Routes[0]
	return Estimate{DistanceMeters: route.Distance, DurationSeconds: route.Duration}, nil
}

func lonLat(p Point) string {
	return strconv.FormatFloat(p.Longitude, 'f', -1, 64) + "," + strconv.FormatFloat(p.Latitude, 'f', -1, 64)
}
