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
	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

const addOrderPath = "/api/v1/routes/2026-10-16-A-1/orders"

func TestRoutePlan_DryRunIsForwardedAndEchoed(t *testing.T) {
	routing := &fakeRouting{}
	rec := do(routePlanRouter(routing), http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16","dryRun":true}`, authHeaderFor(t, "OPERATOR"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, routing.lastReq.GetDryRun())
	assert.Contains(t, rec.Body.String(), `"dryRun":true`)

	routing = &fakeRouting{}
	rec = do(routePlanRouter(routing), http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, authHeaderFor(t, "OPERATOR"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, routing.lastReq.GetDryRun(), "no dryRun in the body must be a real plan")
	assert.NotContains(t, rec.Body.String(), "dryRun", "a real plan response keeps its previous shape")
}

// proposedDrivers names drivers by id, like Driver Service does for the fleet.
type proposedDrivers struct{ fakeDriverClient }

func (f *proposedDrivers) ListDrivers(context.Context, grpcclient.CallMeta, *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	return &driversv1.ListDriversResponse{Drivers: []*driversv1.DriverListItem{{Id: "d-9", FirstName: "Ana", LastName: "Ruiz"}}}, nil
}

func TestRoutePlan_DryRunNamesTheProposedDrivers(t *testing.T) {
	routing := &fakeRouting{resp: &routingv1.GeneratePlanResponse{PlanId: "plan-x", Routes: []*routingv1.PlannedRoute{
		{RouteId: "r-1", DriverId: "d-9", OrderCount: 1},
		{RouteId: "r-2", OrderCount: 1}, // no driver left
	}}}
	h := newRouterWithSignup(&fakeAuth{}, httpapi.Options{Routing: routing, Drivers: &proposedDrivers{}})

	rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16","dryRun":true}`, authHeaderFor(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Routes []struct {
			DriverID   string `json:"driverId"`
			DriverName string `json:"driverName"`
		} `json:"routes"`
	}
	decodeJSON(t, rec.Body.Bytes(), &body)
	require.Len(t, body.Routes, 2)
	assert.Equal(t, "Ana Ruiz", body.Routes[0].DriverName)
	assert.Empty(t, body.Routes[1].DriverID)
	assert.Empty(t, body.Routes[1].DriverName)
}

func TestAddOrderToRoute_OnlyStaff(t *testing.T) {
	routing := &fakeRouting{}
	h := routePlanRouter(routing)

	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, addOrderPath, `{"orderId":"o-1"}`, nil).Code)
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		assert.Equal(t, http.StatusForbidden, do(h, http.MethodPost, addOrderPath, `{"orderId":"o-1"}`, authHeaderFor(t, role)).Code, role)
	}
	assert.Zero(t, routing.calls)
}

func TestAddOrderToRoute_Returns201WithTheNewStop(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR"} {
		routing := &fakeRouting{}
		rec := do(routePlanRouter(routing), http.MethodPost, addOrderPath, `{"orderId":"o-1"}`, authHeaderFor(t, role))

		require.Equal(t, http.StatusCreated, rec.Code, role)
		assert.JSONEq(t, `{"routeId":"2026-10-16-A-1","stop":{"orderId":"o-1","address":"Calle Nueva 1","sequence":4}}`, rec.Body.String(), role)
		assert.Equal(t, "2026-10-16-A-1", routing.lastAdd.GetRouteId())
		assert.Equal(t, "o-1", routing.lastAdd.GetOrderId())
		assert.Equal(t, role, routing.lastMeta.Role, "the identity travels to the Routing Service")
	}
}

func TestAddOrderToRoute_BlankOrderIsRejectedWithoutCall(t *testing.T) {
	routing := &fakeRouting{}
	rec := do(routePlanRouter(routing), http.MethodPost, addOrderPath, `{"orderId":"  "}`, authHeaderFor(t, "ADMIN"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, routing.calls)
}

func TestAddOrderToRoute_RoutingErrorsMapToHTTPStatus(t *testing.T) {
	tests := []struct {
		code codes.Code
		want int
	}{
		{codes.NotFound, http.StatusNotFound},
		{codes.FailedPrecondition, http.StatusUnprocessableEntity},
		{codes.Unavailable, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		routing := &fakeRouting{err: status.Error(tt.code, "boom")}
		rec := do(routePlanRouter(routing), http.MethodPost, addOrderPath, `{"orderId":"o-1"}`, authHeaderFor(t, "ADMIN"))
		assert.Equal(t, tt.want, rec.Code, tt.code.String())
	}
}
