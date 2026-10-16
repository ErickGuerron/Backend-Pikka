// Package osrm adapts an OSRM HTTP server (osrm-routed) to estimate road distance and driving time.
//
// The server holds one map region (Ecuador in the MVP). Its base URL is configuration; the
// adapter knows nothing about regions, so adding a country means adding a server, not code.
package osrm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// snapRadiusMeters is the largest distance a point may be from a road and still be snapped to it.
// GPS fixes and geocoded addresses are a few tens of meters off; anything beyond this is not on the map.
const snapRadiusMeters = "1000"

// maxBodyBytes caps the response read from OSRM. A route without geometry is a few hundred bytes.
const maxBodyBytes = 64 << 10

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
// both map to domain.ErrNoRoute.
func (e *Estimator) Estimate(ctx context.Context, from, to domain.Point) (domain.Estimate, error) {
	if !from.Valid() || !to.Valid() {
		return domain.Estimate{}, domain.ErrInvalidPoint
	}

	// OSRM expects longitude,latitude pairs separated by a literal ';'. The query is built from
	// constants and validated numbers only, so it is written as-is (url.Values would escape ';').
	coords := lonLat(from) + ";" + lonLat(to)
	radiuses := snapRadiusMeters + ";" + snapRadiusMeters
	endpoint := e.baseURL + "/route/v1/driving/" + coords + "?overview=false&radiuses=" + radiuses

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.Estimate{}, fmt.Errorf("osrm request: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return domain.Estimate{}, fmt.Errorf("osrm call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return domain.Estimate{}, fmt.Errorf("osrm read: %w", err)
	}

	var parsed routeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Some failures are not JSON at all; report the status when it is not OK.
		if resp.StatusCode != http.StatusOK {
			return domain.Estimate{}, fmt.Errorf("osrm status %d", resp.StatusCode)
		}
		return domain.Estimate{}, fmt.Errorf("osrm decode: %w", err)
	}
	// NoSegment: the point is off the map or far from any road. OSRM answers it with HTTP 400,
	// so the body code decides before the status does. Both codes mean "no route".
	if parsed.Code == "NoRoute" || parsed.Code == "NoSegment" {
		return domain.Estimate{}, domain.ErrNoRoute
	}
	if resp.StatusCode != http.StatusOK {
		return domain.Estimate{}, fmt.Errorf("osrm status %d", resp.StatusCode)
	}
	if len(parsed.Routes) == 0 {
		return domain.Estimate{}, domain.ErrNoRoute
	}
	if parsed.Code != "Ok" {
		return domain.Estimate{}, fmt.Errorf("osrm code %q", parsed.Code)
	}
	route := parsed.Routes[0]
	return domain.Estimate{DistanceMeters: route.Distance, DurationSeconds: route.Duration}, nil
}

func lonLat(p domain.Point) string {
	return strconv.FormatFloat(p.Longitude, 'f', -1, 64) + "," + strconv.FormatFloat(p.Latitude, 'f', -1, 64)
}
