package http_test

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/libs/osrm"
)

func freshFix() *trackingv1.GetDriverPositionResponse {
	return &trackingv1.GetDriverPositionResponse{Latitude: -0.19, Longitude: -78.49, RecordedAt: "2026-10-16T11:59:30Z"}
}

func TestTrackingSnapshot_DrawsTheRoadFromTheDriverToTheCustomersDeliveryPoint(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	tracking := &positionTrackingFake{pos: freshFix()}
	planner := &fakePlanner{trip: osrm.Trip{Geometry: []osrm.Point{{Latitude: -0.19, Longitude: -78.49}, {Latitude: -0.185, Longitude: -78.485}}}}
	opts := trackingOptions(orders, drivers, tracking)
	opts.Planner = planner

	body := snapshotJSON(t, routerFor(opts, io.Discard))

	assert.Equal(t, []any{[]any{-0.19, -78.49}, []any{-0.185, -78.485}}, body["road"], "road is a list of [lat, lon] pairs")
	assert.Equal(t, osrm.Point{Latitude: -0.19, Longitude: -78.49}, planner.origin, "the road starts at the driver")
	assert.Equal(t, []osrm.Point{{Latitude: trackedLatitude, Longitude: trackedLongitude}}, planner.stops,
		"the road only goes to this customer's delivery point, never through other orders")
}

func TestTrackingSnapshot_RoadFailureDegradesToNullAndStillServes(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	tracking := &positionTrackingFake{pos: freshFix()}
	opts := trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, tracking)
	opts.Planner = &fakePlanner{err: errors.New("osrm down")}

	body := snapshotJSON(t, routerFor(opts, io.Discard))

	assert.Contains(t, body, "road")
	assert.Nil(t, body["road"])
	assert.Equal(t, trackedOrderID, body["order_id"])
}

func TestTrackingSnapshot_NoRoadWithoutAFreshDriverPosition(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_PENDING, "")}
	planner := &fakePlanner{}
	opts := trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, &positionTrackingFake{})
	opts.Planner = planner

	body := snapshotJSON(t, routerFor(opts, io.Discard))

	assert.Nil(t, body["road"])
	assert.Equal(t, 0, planner.calls)
}

func TestTrackingSnapshot_NoRoadWhenRoutePlanningIsNotConfigured(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	tracking := &positionTrackingFake{pos: freshFix()}
	opts := trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, tracking)
	opts.Planner = nil

	body := snapshotJSON(t, routerFor(opts, io.Discard))

	require.Contains(t, body, "road")
	assert.Nil(t, body["road"])
}
