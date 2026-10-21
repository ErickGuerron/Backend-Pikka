package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

const routePlanPath = "/api/v1/routes/plan"

// fakeRouting counts calls and records the identity and request it received.
type fakeRouting struct {
	calls    int
	lastMeta grpcclient.CallMeta
	lastReq  *routingv1.GeneratePlanRequest
	resp     *routingv1.GeneratePlanResponse
	err      error
}

func (f *fakeRouting) GeneratePlan(_ context.Context, m grpcclient.CallMeta, req *routingv1.GeneratePlanRequest) (*routingv1.GeneratePlanResponse, error) {
	f.calls++
	f.lastMeta = m
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &routingv1.GeneratePlanResponse{PlanId: "plan-2026-10-16", Routes: []*routingv1.PlannedRoute{
		{RouteId: "2026-10-16/A", DriverId: "d-1", DriverName: "Luis Andrade", OrderCount: 2, Stops: []*routingv1.PlannedStop{
			{OrderId: "o-1", Address: "Av. Amazonas 100", Sequence: 1},
			{OrderId: "o-2", Address: "Calle 9 de Octubre 20", Sequence: 2},
		}},
		{RouteId: "2026-10-16/B", DriverId: "d-2", DriverName: "Jorge Velez", OrderCount: 1, Stops: []*routingv1.PlannedStop{
			{OrderId: "o-3", Address: "Av. 6 de Diciembre 50", Sequence: 1},
		}},
	}}, nil
}

func routePlanRouter(r httpapi.RoutingClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Routing: r})
}

func authHeaderFor(t *testing.T, role string) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)}
}

func TestRoutePlan_WithoutToken_Returns401WithoutCall(t *testing.T) {
	routing := &fakeRouting{}
	h := routePlanRouter(routing)

	rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, routing.calls)
}

// Only ADMIN and OPERATOR may plan. StaffOnly rejects other roles before the handler runs.
func TestRoutePlan_RolesOtherThanAdminAndOperator_Returns403WithoutCall(t *testing.T) {
	routing := &fakeRouting{}
	h := routePlanRouter(routing)

	for _, role := range []string{"DRIVER", "CUSTOMER"} {
		rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, authHeaderFor(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code, role)
	}
	assert.Zero(t, routing.calls)
}

func TestRoutePlan_AdminAndOperator_Returns200WithRoutesAndForwardsIdentity(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR"} {
		t.Run(role, func(t *testing.T) {
			routing := &fakeRouting{}
			h := routePlanRouter(routing)

			rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, authHeaderFor(t, role))

			require.Equal(t, http.StatusOK, rec.Code)
			assert.JSONEq(t, `{"planId":"plan-2026-10-16","unassignedOrderIds":[],"routes":[
				{"routeId":"2026-10-16/A","driverId":"d-1","driverName":"Luis Andrade","orderCount":2,"stops":[
					{"orderId":"o-1","address":"Av. Amazonas 100","sequence":1},
					{"orderId":"o-2","address":"Calle 9 de Octubre 20","sequence":2}]},
				{"routeId":"2026-10-16/B","driverId":"d-2","driverName":"Jorge Velez","orderCount":1,"stops":[
					{"orderId":"o-3","address":"Av. 6 de Diciembre 50","sequence":1}]}]}`, rec.Body.String())
			require.Equal(t, 1, routing.calls)
			assert.Equal(t, "2026-10-16", routing.lastReq.GetDeliveryDate())
			assert.Equal(t, "u-"+role, routing.lastMeta.UserID, "the caller identity is forwarded")
			assert.Equal(t, role, routing.lastMeta.Role, "the caller role is forwarded")
		})
	}
}

func TestRoutePlan_NoRoutesPlanned_ReturnsEmptyArrayNotNull(t *testing.T) {
	routing := &fakeRouting{resp: &routingv1.GeneratePlanResponse{}}
	h := routePlanRouter(routing)

	rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, authHeaderFor(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"planId":"","unassignedOrderIds":[],"routes":[]}`, rec.Body.String())
}

func TestRoutePlan_InvalidBody_Returns400WithoutCall(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"malformed JSON", `{"deliveryDate":`, "malformed JSON body"},
		{"deliveryDate missing", `{}`, "deliveryDate is required"},
		{"deliveryDate blank", `{"deliveryDate":"   "}`, "deliveryDate is required"},
		{"deliveryDate is a number", `{"deliveryDate":20261016}`, "malformed JSON body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routing := &fakeRouting{}
			h := routePlanRouter(routing)

			rec := do(h, http.MethodPost, routePlanPath, tc.body, authHeaderFor(t, "ADMIN"))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			e := decodeError(t, rec)
			assert.Equal(t, "INVALID_ARGUMENT", e.Code)
			assert.Equal(t, tc.message, e.Message)
			assert.Zero(t, routing.calls)
		})
	}
}

func TestRoutePlan_WrongContentType_Returns415WithoutCall(t *testing.T) {
	routing := &fakeRouting{}
	h := routePlanRouter(routing)
	req := httptest.NewRequest(http.MethodPost, routePlanPath, strings.NewReader(`{"deliveryDate":"2026-10-16"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", authHeaderFor(t, "ADMIN")["Authorization"])
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.Zero(t, routing.calls)
}

// Each Routing Service code reaches the caller through the same error table as the rest of the API. None returns 200.
func TestRoutePlan_RoutingErrors_MapToErrorEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantName string
	}{
		{"invalid date rejected by routing", status.Error(codes.InvalidArgument, "delivery date must be YYYY-MM-DD"), http.StatusBadRequest, "INVALID_ARGUMENT"},
		{"no driver available for a route", status.Error(codes.FailedPrecondition, "route 2026-10-16/A: no driver available"), http.StatusUnprocessableEntity, "FAILED_PRECONDITION"},
		{"routing denies the role", status.Error(codes.PermissionDenied, "role not allowed"), http.StatusForbidden, "FORBIDDEN"},
		{"routing rejects the identity", status.Error(codes.Unauthenticated, "missing role"), http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"routing unavailable", status.Error(codes.Unavailable, "routing unavailable"), http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE"},
		{"routing timeout", status.Error(codes.DeadlineExceeded, "deadline"), http.StatusGatewayTimeout, "UPSTREAM_TIMEOUT"},
		{"routing internal failure", status.Error(codes.Internal, "boom"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routing := &fakeRouting{err: tc.err}
			h := routePlanRouter(routing)

			rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, authHeaderFor(t, "OPERATOR"))

			assert.Equal(t, tc.wantCode, rec.Code)
			assert.Equal(t, tc.wantName, decodeError(t, rec).Code)
			assert.Equal(t, 1, routing.calls)
		})
	}
}

// Without a routing client the route does not exist: the Gateway never invents a plan.
func TestRoutePlan_RouteAbsentWithoutRoutingClient(t *testing.T) {
	h := newRouterWithSignup(&fakeAuth{}, httpapi.Options{})

	rec := do(h, http.MethodPost, routePlanPath, `{"deliveryDate":"2026-10-16"}`, authHeaderFor(t, "ADMIN"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
