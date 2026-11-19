package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

const activeRoutesPath = "/api/v1/tracking/routes/active"

// routedOrders serves a fixed set of orders and applies the status and day filters like Order Service.
type routedOrders struct {
	fakeOrderClient
	err   error
	asked []*ordersv1.ListOrdersRequest
}

func trackedOrder(id, route, day string, seq int32, st ordersv1.OrderStatus) *ordersv1.Order {
	return &ordersv1.Order{Id: id, Address: "Calle " + id, RouteId: route, DeliveryDate: day, StopSequence: seq, Status: st}
}

func (f *routedOrders) ListOrders(_ context.Context, _ grpcclient.CallMeta, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	f.asked = append(f.asked, req)
	if f.err != nil {
		return nil, f.err
	}
	const (
		assigned  = ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED
		transit   = ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT
		delivered = ordersv1.OrderStatus_ORDER_STATUS_DELIVERED
	)
	all := []*ordersv1.Order{
		trackedOrder("o-5", "r-1", "2026-11-18", 3, ordersv1.OrderStatus_ORDER_STATUS_CANCELLED),
		trackedOrder("o-4", "", "2026-11-18", 0, ordersv1.OrderStatus_ORDER_STATUS_PENDING),
		trackedOrder("o-3", "r-2", "2026-11-17", 1, delivered),
		trackedOrder("o-2", "r-1", "2026-11-18", 2, delivered),
		trackedOrder("o-1", "r-1", "2026-11-18", 1, transit),
		trackedOrder("o-6", "r-3", "2026-11-18", 1, assigned),
	}
	var out []*ordersv1.Order
	for _, o := range all {
		if req.GetStatus() != ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED && o.GetStatus() != req.GetStatus() {
			continue
		}
		if req.GetDeliveryDate() != "" && o.GetDeliveryDate() != req.GetDeliveryDate() {
			continue
		}
		out = append(out, o)
	}
	return &ordersv1.ListOrdersResponse{Orders: out}, nil
}

type routedDrivers struct{ fakeDriverClient }

func (f *routedDrivers) GetDriverByRouteID(_ context.Context, _ grpcclient.CallMeta, req *driversv1.GetDriverByRouteIDRequest) (*driversv1.GetDriverByRouteIDResponse, error) {
	if req.GetRouteId() != "r-1" {
		return nil, status.Error(codes.NotFound, "no driver")
	}
	return &driversv1.GetDriverByRouteIDResponse{DriverId: "d-1"}, nil
}

func (f *routedDrivers) ListDrivers(context.Context, grpcclient.CallMeta, *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	return &driversv1.ListDriversResponse{Drivers: []*driversv1.DriverListItem{
		{Id: "d-1", FirstName: "Lucas", LastName: "Martinez"},
	}}, nil
}

func activeRoutesRouter(o httpapi.OrderClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Orders: o, Drivers: &routedDrivers{}})
}

func TestActiveRoutes_OnlyStaff(t *testing.T) {
	h := activeRoutesRouter(&routedOrders{})

	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, activeRoutesPath, "", nil).Code)
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		assert.Equal(t, http.StatusForbidden, do(h, http.MethodGet, activeRoutesPath, "", staffHeader(t, role)).Code, role)
	}
}

func TestActiveRoutes_ListsOpenRoutesWithStopsInPlannedOrder(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR"} {
		orders := &routedOrders{}
		rec := do(activeRoutesRouter(orders), http.MethodGet, activeRoutesPath, "", staffHeader(t, role))

		require.Equal(t, http.StatusOK, rec.Code, role)
		// r-2 is finished and o-5 is cancelled, so neither appears. o-2 (already delivered) stays as a stop of r-1.
		assert.JSONEq(t, `[
			{"id":"r-1","driverName":"Lucas Martinez","stops":[
				{"orderId":"o-1","address":"Calle o-1","sequence":1,"status":"IN_TRANSIT"},
				{"orderId":"o-2","address":"Calle o-2","sequence":2,"status":"DELIVERED"}]},
			{"id":"r-3","driverName":"Sin repartidor","stops":[
				{"orderId":"o-6","address":"Calle o-6","sequence":1,"status":"PENDING"}]}]`, rec.Body.String(), role)
	}
}

func TestActiveRoutes_ReadsOnlyDaysWithOpenWork(t *testing.T) {
	orders := &routedOrders{}
	do(activeRoutesRouter(orders), http.MethodGet, activeRoutesPath, "", staffHeader(t, "ADMIN"))

	// 3 open-status queries plus one query for the only day with open work. 2026-11-17 is never read.
	require.Len(t, orders.asked, 4)
	assert.Equal(t, "2026-11-18", orders.asked[3].GetDeliveryDate())
}

func TestActiveRoutes_NoOpenRoutesIsAnEmptyArray(t *testing.T) {
	h := newRouterWithSignup(&fakeAuth{}, httpapi.Options{Orders: &emptyOrders{}, Drivers: &routedDrivers{}})
	rec := do(h, http.MethodGet, activeRoutesPath, "", staffHeader(t, "ADMIN"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[]`, rec.Body.String())
}

func TestActiveRoutes_UpstreamFailureIsMapped(t *testing.T) {
	h := activeRoutesRouter(&routedOrders{err: status.Error(codes.Unavailable, "orders down")})
	rec := do(h, http.MethodGet, activeRoutesPath, "", staffHeader(t, "ADMIN"))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// positionedTracking knows where the driver d-1 is; any other driver has no fresh position.
type positionedTracking struct{ err error }

func (f positionedTracking) RecordPosition(context.Context, grpcclient.CallMeta, *trackingv1.RecordPositionRequest) (*trackingv1.RecordPositionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used")
}

func (f positionedTracking) EstimateArrival(context.Context, grpcclient.CallMeta, *trackingv1.EstimateArrivalRequest) (*trackingv1.EstimateArrivalResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used")
}

func (f positionedTracking) GetDriverPosition(_ context.Context, _ grpcclient.CallMeta, req *trackingv1.GetDriverPositionRequest) (*trackingv1.GetDriverPositionResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	if req.GetDriverId() != "d-1" {
		return nil, status.Error(codes.NotFound, "no fresh position")
	}
	return &trackingv1.GetDriverPositionResponse{Latitude: -0.18, Longitude: -78.47, RecordedAt: "2026-11-18T12:00:00Z"}, nil
}

func trackedRouter(tracking httpapi.TrackingClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Orders: &routedOrders{}, Drivers: &routedDrivers{}, Tracking: tracking})
}

func TestActiveRoutes_AddsTheDriversFreshPosition(t *testing.T) {
	rec := do(trackedRouter(positionedTracking{}), http.MethodGet, activeRoutesPath, "", staffHeader(t, "OPERATOR"))

	require.Equal(t, http.StatusOK, rec.Code)
	var routes []struct {
		ID             string `json:"id"`
		DriverPosition *struct {
			Latitude   float64 `json:"latitude"`
			Longitude  float64 `json:"longitude"`
			RecordedAt string  `json:"recordedAt"`
		} `json:"driverPosition"`
	}
	decodeJSON(t, rec.Body.Bytes(), &routes)
	require.Len(t, routes, 2)
	require.NotNil(t, routes[0].DriverPosition, "r-1 is run by d-1, who has a fresh position")
	assert.InDelta(t, -0.18, routes[0].DriverPosition.Latitude, 1e-9)
	assert.InDelta(t, -78.47, routes[0].DriverPosition.Longitude, 1e-9)
	assert.Equal(t, "2026-11-18T12:00:00Z", routes[0].DriverPosition.RecordedAt)
	assert.Nil(t, routes[1].DriverPosition, "r-3 has no driver, so there is nothing to locate")
}

func TestActiveRoutes_NoFreshPositionOrTrackingFailureHidesTheMarkerButKeepsTheRoutes(t *testing.T) {
	for name, tracking := range map[string]httpapi.TrackingClient{
		"driver without fix": positionedTracking{err: status.Error(codes.NotFound, "none")},
		"tracking down":      positionedTracking{err: status.Error(codes.Unavailable, "down")},
	} {
		rec := do(trackedRouter(tracking), http.MethodGet, activeRoutesPath, "", staffHeader(t, "ADMIN"))

		require.Equal(t, http.StatusOK, rec.Code, name)
		assert.NotContains(t, rec.Body.String(), "driverPosition", name)
		assert.Contains(t, rec.Body.String(), `"id":"r-1"`, name)
	}
}
