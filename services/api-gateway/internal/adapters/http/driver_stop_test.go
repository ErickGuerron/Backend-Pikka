package http_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

const (
	stopPath   = "/api/v1/drivers/me/routes/" + trackedRouteID + "/stops/" + trackedOrderID + "/status"
	driverUser = "u-driver"
)

func driverStopHeaders(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": "Bearer " + token(t, driverUser, "DRIVER")}
}

func stopRouter(orders *trackingOrdersFake, drivers *routeDriversFake) http.Handler {
	return routerFor(trackingOptions(orders, drivers, nil), io.Discard)
}

func advanceOK(status ordersv1.OrderStatus) *ordersv1.AdvanceRouteStopResponse {
	return &ordersv1.AdvanceRouteStopResponse{Order: &ordersv1.Order{Id: trackedOrderID, RouteId: trackedRouteID, Status: status}}
}

func TestDriverStop_PickedUpAdvancesOwnRouteAndReturnsNewStatus(t *testing.T) {
	orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)}
	drivers := &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID}
	h := stopRouter(orders, drivers)

	rec := do(h, http.MethodPatch, stopPath, `{"status":"PICKED_UP"}`, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"stopId":"`+trackedOrderID+`","status":"PICKED_UP"}`, rec.Body.String())
	require.NotNil(t, orders.advanceReq)
	assert.Equal(t, trackedRouteID, orders.advanceReq.GetRouteId())
	assert.Equal(t, trackedOrderID, orders.advanceReq.GetOrderId())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP, orders.advanceReq.GetTargetStatus())
	assert.Equal(t, "DRIVER", orders.gotMeta.Role)
}

func TestDriverStop_InTransitAdvances(t *testing.T) {
	orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT)}
	drivers := &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID}

	rec := do(stopRouter(orders, drivers), http.MethodPatch, stopPath, `{"status":"IN_TRANSIT"}`, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"stopId":"`+trackedOrderID+`","status":"IN_TRANSIT"}`, rec.Body.String())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT, orders.advanceReq.GetTargetStatus())
}

// Solo PICKED_UP e IN_TRANSIT. DELIVERED y NOT_DELIVERED van por el registro de entrega, no por aquí.
func TestDriverStop_OtherStatusesAreInvalidArgumentAndNeverCallOrders(t *testing.T) {
	for _, body := range []string{`{"status":"DELIVERED"}`, `{"status":"NOT_DELIVERED"}`, `{"status":"picked_up"}`, `{"status":""}`, `{}`, `not json`} {
		orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)}
		drivers := &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID}

		rec := do(stopRouter(orders, drivers), http.MethodPatch, stopPath, body, driverStopHeaders(t))

		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Equal(t, "INVALID_ARGUMENT", decodeError(t, rec).Code, body)
		assert.Nil(t, orders.advanceReq, body)
	}
}

func TestDriverStop_RouteOfAnotherDriverIsNotFound(t *testing.T) {
	orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)}
	drivers := &routeDriversFake{byRoute: "drv-other", userDriver: trackedDriverID}

	rec := do(stopRouter(orders, drivers), http.MethodPatch, stopPath, `{"status":"PICKED_UP"}`, driverStopHeaders(t))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Nil(t, orders.advanceReq, "a route that is not the driver's is never advanced")
}

// Una ruta ajena y una ruta sin repartidor responden igual: no se revela qué rutas existen.
func TestDriverStop_UnknownAndForeignRoutesReturnTheSameError(t *testing.T) {
	foreign := do(stopRouter(&trackingOrdersFake{}, &routeDriversFake{byRoute: "drv-other", userDriver: trackedDriverID}),
		http.MethodPatch, stopPath, `{"status":"PICKED_UP"}`, driverStopHeaders(t))
	unknown := do(stopRouter(&trackingOrdersFake{}, &routeDriversFake{byRoute: "", userDriver: trackedDriverID}),
		http.MethodPatch, stopPath, `{"status":"PICKED_UP"}`, driverStopHeaders(t))

	require.Equal(t, http.StatusNotFound, foreign.Code)
	require.Equal(t, http.StatusNotFound, unknown.Code)
	assert.Equal(t, decodeError(t, foreign).Code, decodeError(t, unknown).Code)
	assert.Equal(t, decodeError(t, foreign).Message, decodeError(t, unknown).Message)
}

func TestDriverStop_OrderNotOnRouteIsNotFoundAndIllegalStepIsUnprocessable(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"order not on route", status.Error(codes.NotFound, "pedido no encontrado"), http.StatusNotFound, "NOT_FOUND"},
		{"illegal transition", status.Error(codes.FailedPrecondition, "transicion no permitida"), http.StatusUnprocessableEntity, "FAILED_PRECONDITION"},
	}
	for _, tc := range cases {
		orders := &trackingOrdersFake{advanceErr: tc.err}
		drivers := &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID}

		rec := do(stopRouter(orders, drivers), http.MethodPatch, stopPath, `{"status":"IN_TRANSIT"}`, driverStopHeaders(t))

		assert.Equal(t, tc.status, rec.Code, tc.name)
		assert.Equal(t, tc.code, decodeError(t, rec).Code, tc.name)
	}
}

func TestDriverStop_NonDriverRolesAre403(t *testing.T) {
	orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)}
	h := stopRouter(orders, &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID})
	for _, role := range []string{"CUSTOMER", "OPERATOR", "ADMIN"} {
		headers := map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)}
		rec := do(h, http.MethodPatch, stopPath, `{"status":"PICKED_UP"}`, headers)
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
	}
	assert.Nil(t, orders.advanceReq)
}
