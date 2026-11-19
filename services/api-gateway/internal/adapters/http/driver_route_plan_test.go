package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/libs/osrm"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// fakePlanner records the origin and stops it receives and answers with a prepared trip.
type fakePlanner struct {
	mu     sync.Mutex
	calls  int
	origin osrm.Point
	stops  []osrm.Point
	trip   osrm.Trip
	err    error
}

func (f *fakePlanner) Optimize(_ context.Context, origin osrm.Point, stops []osrm.Point) (osrm.Trip, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.origin = origin
	f.stops = stops
	if f.err != nil {
		return osrm.Trip{}, f.err
	}
	return f.trip, nil
}

// planStops has one delivered stop, so the planner must only see the two active ones, in sequence.
func planStops() *ordersv1.ListDriverRouteStopsResponse {
	return &ordersv1.ListDriverRouteStopsResponse{Stops: []*ordersv1.DriverRouteStop{
		{OrderId: "o-1", Sequence: 1, Address: "Av. Amazonas 100", Latitude: -0.18, Longitude: -78.48,
			Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED},
		{OrderId: "o-2", Sequence: 2, Address: "Calle Larga 5", Latitude: -0.19, Longitude: -78.49,
			Status: ordersv1.OrderStatus_ORDER_STATUS_DELIVERED},
		{OrderId: "o-3", Sequence: 3, Address: "Calle Juan Leon 9", Latitude: -0.21, Longitude: -78.5,
			Status: ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP},
	}}
}

type planBody struct {
	Order           []string    `json:"order"`
	DistanceMeters  float64     `json:"distanceMeters"`
	DurationSeconds float64     `json:"durationSeconds"`
	Geometry        [][]float64 `json:"geometry"`
}

func planGet(t *testing.T, opts httpapi.Options, role, query string) *httptest.ResponseRecorder {
	t.Helper()
	router := routerFor(opts, io.Discard)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/drivers/me/route/plan"+query, nil)
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+token(t, "u-driver", role))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func planOptions(orders httpapi.OrderClient, planner httpapi.RoutePlanner) httpapi.Options {
	opts := trackingOptions(orders, &readRouteDrivers{routeID: readRouteID}, nil)
	opts.Planner = planner
	return opts
}

func TestDriverRoutePlan_OrdersActiveStopsByTheBestRoute(t *testing.T) {
	orders := &readStopsOrders{res: planStops()}
	planner := &fakePlanner{trip: osrm.Trip{
		Order:           []int{1, 0}, // OSRM visits the second active stop (o-3) first.
		DistanceMeters:  2400,
		DurationSeconds: 420,
		Geometry:        []osrm.Point{{Latitude: -0.2, Longitude: -78.5}, {Latitude: -0.18, Longitude: -78.48}},
	}}

	rec := planGet(t, planOptions(orders, planner), "DRIVER", "?lat=-0.2&lon=-78.5")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, osrm.Point{Latitude: -0.2, Longitude: -78.5}, planner.origin, "the origin is the phone's position")
	require.Equal(t, []osrm.Point{{Latitude: -0.18, Longitude: -78.48}, {Latitude: -0.21, Longitude: -78.5}}, planner.stops,
		"only the active stops, in sequence, are planned")
	var body planBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, []string{"o-3", "o-1"}, body.Order, "the response carries stop ids in the best order")
	assert.InDelta(t, 2400, body.DistanceMeters, 1e-9)
	assert.InDelta(t, 420, body.DurationSeconds, 1e-9)
	assert.Equal(t, [][]float64{{-0.2, -78.5}, {-0.18, -78.48}}, body.Geometry, "geometry is [lat, lon] for the map")
}

func TestDriverRoutePlan_NeedsTheDriverPositionAsNumbers(t *testing.T) {
	for _, query := range []string{"", "?lat=-0.2", "?lon=-78.5", "?lat=abc&lon=-78.5", "?lat=91&lon=-78.5"} {
		orders := &readStopsOrders{res: planStops()}
		planner := &fakePlanner{}

		rec := planGet(t, planOptions(orders, planner), "DRIVER", query)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "query %q", query)
		assert.Equal(t, 0, planner.calls, "query %q must not reach the planner", query)
	}
}

func TestDriverRoutePlan_NoActiveStopsIsAnEmptyPlanWithoutAPlanCall(t *testing.T) {
	orders := &readStopsOrders{res: &ordersv1.ListDriverRouteStopsResponse{Stops: []*ordersv1.DriverRouteStop{
		{OrderId: "o-2", Sequence: 1, Latitude: -0.19, Longitude: -78.49, Status: ordersv1.OrderStatus_ORDER_STATUS_DELIVERED},
	}}}
	planner := &fakePlanner{}

	rec := planGet(t, planOptions(orders, planner), "DRIVER", "?lat=-0.2&lon=-78.5")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 0, planner.calls)
	var body planBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Order)
	assert.Empty(t, body.Geometry)
}

func TestDriverRoutePlan_NoRouteAssignedIsAnEmptyPlan(t *testing.T) {
	orders := &readStopsOrders{res: planStops()}
	planner := &fakePlanner{}
	opts := trackingOptions(orders, &readRouteDrivers{noProfile: true}, nil)
	opts.Planner = planner

	rec := planGet(t, opts, "DRIVER", "?lat=-0.2&lon=-78.5")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 0, planner.calls)
	assert.Equal(t, 0, orders.calls)
}

func TestDriverRoutePlan_NoRoadRouteIsUnprocessable(t *testing.T) {
	orders := &readStopsOrders{res: planStops()}
	planner := &fakePlanner{err: osrm.ErrNoRoute}

	rec := planGet(t, planOptions(orders, planner), "DRIVER", "?lat=-0.2&lon=-78.5")

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
}

func TestDriverRoutePlan_PlannerDownIsServiceUnavailableNotA500(t *testing.T) {
	// OSRM not running (the osrm profile is off) surfaces as a connection error: the driver app must be able
	// to say "route planner unavailable" instead of a generic failure.
	orders := &readStopsOrders{res: planStops()}
	planner := &fakePlanner{err: errors.New("osrm trip call: connection refused")}

	rec := planGet(t, planOptions(orders, planner), "DRIVER", "?lat=-0.2&lon=-78.5")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
}

func TestDriverRoutePlan_WithoutAPlannerIsUnavailable(t *testing.T) {
	orders := &readStopsOrders{res: planStops()}

	rec := planGet(t, planOptions(orders, nil), "DRIVER", "?lat=-0.2&lon=-78.5")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
}

func TestDriverRoutePlan_OnlyTheDriverRoleGetsIt(t *testing.T) {
	orders := &readStopsOrders{res: planStops()}
	planner := &fakePlanner{}

	assert.Equal(t, http.StatusForbidden, planGet(t, planOptions(orders, planner), "ADMIN", "?lat=-0.2&lon=-78.5").Code)
	assert.Equal(t, http.StatusUnauthorized, planGet(t, planOptions(orders, planner), "", "?lat=-0.2&lon=-78.5").Code)
	assert.Equal(t, 0, planner.calls)
}
