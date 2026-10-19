package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// positionTrackingFake responde la estimación y la última posición del repartidor.
type positionTrackingFake struct {
	httpapi.TrackingClient
	eta          *trackingv1.EstimateArrivalResponse
	pos          *trackingv1.GetDriverPositionResponse
	posErr       error
	posCalls     int
	gotPosDriver string
}

func (f *positionTrackingFake) EstimateArrival(_ context.Context, _ grpcclient.CallMeta, _ *trackingv1.EstimateArrivalRequest) (*trackingv1.EstimateArrivalResponse, error) {
	if f.eta == nil {
		return nil, status.Error(codes.NotFound, "no position")
	}
	return f.eta, nil
}

func (f *positionTrackingFake) GetDriverPosition(_ context.Context, _ grpcclient.CallMeta, req *trackingv1.GetDriverPositionRequest) (*trackingv1.GetDriverPositionResponse, error) {
	f.posCalls++
	f.gotPosDriver = req.GetDriverId()
	if f.posErr != nil {
		return nil, f.posErr
	}
	return f.pos, nil
}

// snapshotJSON pide el snapshot y lo devuelve como mapa genérico, para comprobar la forma exacta del JSON.
func snapshotJSON(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestTrackingSnapshot_IncludesTheFreshDriverPosition(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	tracking := &positionTrackingFake{pos: &trackingv1.GetDriverPositionResponse{Latitude: -0.19, Longitude: -78.49, RecordedAt: "2026-10-16T11:59:30Z"}}
	h := routerFor(trackingOptions(orders, drivers, tracking), io.Discard)

	body := snapshotJSON(t, h)

	require.Contains(t, body, "driver_position")
	pos, ok := body["driver_position"].(map[string]any)
	require.True(t, ok, "driver_position must be an object when the fix is fresh")
	assert.InDelta(t, -0.19, pos["latitude"], 1e-9)
	assert.InDelta(t, -78.49, pos["longitude"], 1e-9)
	assert.Equal(t, "2026-10-16T11:59:30Z", pos["recorded_at"])
	assert.Equal(t, trackedDriverID, tracking.gotPosDriver, "the position is asked for the route's driver only")
}

func TestTrackingSnapshot_NoFreshPositionIsNullAndTheSnapshotStillServes(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	tracking := &positionTrackingFake{posErr: status.Error(codes.NotFound, "stale")}
	h := routerFor(trackingOptions(orders, drivers, tracking), io.Discard)

	body := snapshotJSON(t, h)

	assert.Contains(t, body, "driver_position")
	assert.Nil(t, body["driver_position"])
	assert.Equal(t, trackedOrderID, body["order_id"])
}

func TestTrackingSnapshot_DriverPositionCarriesNoIdentity(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	tracking := &positionTrackingFake{pos: &trackingv1.GetDriverPositionResponse{Latitude: -0.19, Longitude: -78.49, RecordedAt: "2026-10-16T11:59:30Z"}}
	h := routerFor(trackingOptions(orders, drivers, tracking), io.Discard)

	rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t))

	assert.NotContains(t, rec.Body.String(), trackedDriverID, "the driver id never reaches the customer")
	assert.NotContains(t, rec.Body.String(), "driver_id")
	assert.NotContains(t, rec.Body.String(), "phone")
}

func TestTrackingSnapshot_OrderWithoutRouteDoesNotAskForAPosition(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_PENDING, "")}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	tracking := &positionTrackingFake{}
	h := routerFor(trackingOptions(orders, drivers, tracking), io.Discard)

	body := snapshotJSON(t, h)

	assert.Nil(t, body["driver_position"])
	assert.Equal(t, 0, tracking.posCalls)
}

func TestTrackingSnapshot_IncludesTheDeliveryPoint(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_PENDING, "")}
	h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, &positionTrackingFake{}), io.Discard)

	body := snapshotJSON(t, h)

	require.Contains(t, body, "delivery_point")
	point, ok := body["delivery_point"].(map[string]any)
	require.True(t, ok, "delivery_point must be an object")
	assert.InDelta(t, trackedLatitude, point["latitude"], 1e-9)
	assert.InDelta(t, trackedLongitude, point["longitude"], 1e-9)
}
