package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

const resultPath = "/api/v1/drivers/me/routes/" + trackedRouteID + "/stops/" + trackedOrderID + "/result"

const resultDeliveredBody = `{"outcome":"DELIVERED","latitude":-0.1807,"longitude":-78.4678}`

// stopsWith devuelve la ruta del repartidor con la parada de trackedOrderID en el estado dado.
func stopsWith(st ordersv1.OrderStatus) *ordersv1.ListDriverRouteStopsResponse {
	return &ordersv1.ListDriverRouteStopsResponse{Stops: []*ordersv1.DriverRouteStop{{OrderId: trackedOrderID, Status: st}}}
}

// resultFakes prepara un pedido IN_TRANSIT en la ruta del repartidor, con el repartidor reservado.
func resultFakes() (*trackingOrdersFake, *routeDriversFake) {
	orders := &trackingOrdersFake{routeStopsRes: stopsWith(ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT)}
	drivers := &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID}
	return orders, drivers
}

func (f *trackingOrdersFake) ListDriverRouteStops(_ context.Context, _ grpcclient.CallMeta, _ *ordersv1.ListDriverRouteStopsRequest) (*ordersv1.ListDriverRouteStopsResponse, error) {
	if f.routeStopsErr != nil {
		return nil, f.routeStopsErr
	}
	return f.routeStopsRes, nil
}

func (f *trackingOrdersFake) RecordRouteStopOutcome(_ context.Context, m grpcclient.CallMeta, req *ordersv1.RecordRouteStopOutcomeRequest) (*ordersv1.RecordRouteStopOutcomeResponse, error) {
	f.outcomeReq, f.gotMeta = req, m
	if f.outcomeErr != nil {
		return nil, f.outcomeErr
	}
	return &ordersv1.RecordRouteStopOutcomeResponse{Order: &ordersv1.Order{Id: trackedOrderID, RouteId: trackedRouteID, Status: req.GetOutcome()}}, nil
}

// RegisterDelivery acepta el registro salvo que el test fije un fallo. Devuelve el resultado del registro.
func (f *routeDriversFake) RegisterDelivery(_ context.Context, _ grpcclient.CallMeta, req *driversv1.RegisterDeliveryRequest) (*driversv1.RegisterDeliveryResponse, error) {
	f.registerReq = req
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	return &driversv1.RegisterDeliveryResponse{StopId: req.GetStopId(), Outcome: req.GetOutcome(), Reason: req.GetReason(), WithinRadius: true}, nil
}

func TestDriverResult_DeliveredRecordsDeliveryThenClosesOrder(t *testing.T) {
	orders, drivers := resultFakes()

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"stopId":"`+trackedOrderID+`","status":"DELIVERED"}`, rec.Body.String())

	require.NotNil(t, drivers.registerReq)
	assert.Equal(t, trackedDriverID, drivers.registerReq.GetDriverId())
	assert.Equal(t, trackedRouteID, drivers.registerReq.GetRouteId())
	assert.Equal(t, trackedOrderID, drivers.registerReq.GetStopId())
	assert.Equal(t, "DELIVERED", drivers.registerReq.GetOutcome())
	assert.Empty(t, drivers.registerReq.GetReason())
	assert.InDelta(t, -0.1807, drivers.registerReq.GetLatitude(), 1e-9)
	assert.InDelta(t, -78.4678, drivers.registerReq.GetLongitude(), 1e-9)
	_, err := time.Parse(time.RFC3339, drivers.registerReq.GetRecordedAt())
	assert.NoError(t, err, "recorded_at is stamped by the gateway as RFC 3339")

	require.NotNil(t, orders.outcomeReq)
	assert.Equal(t, trackedRouteID, orders.outcomeReq.GetRouteId())
	assert.Equal(t, trackedOrderID, orders.outcomeReq.GetOrderId())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_DELIVERED, orders.outcomeReq.GetOutcome())
	assert.Equal(t, "DRIVER", orders.gotMeta.Role)
}

func TestDriverResult_NotDeliveredForwardsClosedReason(t *testing.T) {
	orders, drivers := resultFakes()
	body := `{"outcome":"NOT_DELIVERED","reason":"CLIENT_ABSENT","latitude":-0.1807,"longitude":-78.4678}`

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, body, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"stopId":"`+trackedOrderID+`","status":"NOT_DELIVERED"}`, rec.Body.String())
	assert.Equal(t, "NOT_DELIVERED", drivers.registerReq.GetOutcome())
	assert.Equal(t, "CLIENT_ABSENT", drivers.registerReq.GetReason())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED, orders.outcomeReq.GetOutcome())
}

// Solo DELIVERED y NOT_DELIVERED. Cualquier otro valor se rechaza antes de tocar un servicio.
func TestDriverResult_OtherOutcomesAreInvalidArgumentAndNeverCallBackends(t *testing.T) {
	for _, body := range []string{`{"outcome":"PICKED_UP"}`, `{"outcome":"IN_TRANSIT"}`, `{"outcome":"delivered"}`, `{"outcome":""}`, `{}`, `not json`} {
		orders, drivers := resultFakes()

		rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, body, driverStopHeaders(t))

		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Equal(t, "INVALID_ARGUMENT", decodeError(t, rec).Code, body)
		assert.Nil(t, drivers.registerReq, body)
		assert.Nil(t, orders.outcomeReq, body)
	}
}

func TestDriverResult_ForeignRouteIsNotFoundAndNothingIsRecorded(t *testing.T) {
	orders, _ := resultFakes()
	drivers := &routeDriversFake{byRoute: "drv-other", userDriver: trackedDriverID}

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Nil(t, drivers.registerReq)
	assert.Nil(t, orders.outcomeReq)
}

// Un paso que no estÃ¡ IN_TRANSIT se rechaza antes de registrar la entrega en Driver Service: asÃ­ el
// registro del repartidor nunca queda aceptado para un pedido que Order Service no podrÃ¡ cerrar.
func TestDriverResult_StopNotInTransitIsRejectedBeforeDriverRecords(t *testing.T) {
	orders, drivers := resultFakes()
	orders.routeStopsRes = stopsWith(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED)

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "FAILED_PRECONDITION", decodeError(t, rec).Code)
	assert.Nil(t, drivers.registerReq)
	assert.Nil(t, orders.outcomeReq)
}

func TestDriverResult_StopMissingFromRouteIsNotFound(t *testing.T) {
	orders, drivers := resultFakes()
	orders.routeStopsRes = &ordersv1.ListDriverRouteStopsResponse{}

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Nil(t, drivers.registerReq)
}

// Driver Service rechaza una entrega fuera del radio (D1): el pedido no se cierra.
func TestDriverResult_DriverRejectionLeavesOrderUntouched(t *testing.T) {
	orders, drivers := resultFakes()
	drivers.registerErr = status.Error(codes.FailedPrecondition, "delivery outside the arrival radius")

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "FAILED_PRECONDITION", decodeError(t, rec).Code)
	assert.Nil(t, orders.outcomeReq)
}

// Si Order Service falla despuÃ©s de que Driver Service aceptÃ³ el registro, la respuesta es de error y
// nunca un 200. El estado queda parcial y se reporta; no se reintenta a ciegas.
func TestDriverResult_OrderFailureAfterDriverAcceptedIsReportedNotSwallowed(t *testing.T) {
	orders, drivers := resultFakes()
	orders.outcomeErr = status.Error(codes.Unavailable, "order service down")

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotNil(t, drivers.registerReq, "the delivery was accepted by driver service before the failure")
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestDriverResult_NonDriverRolesAre403(t *testing.T) {
	orders, drivers := resultFakes()
	h := stopRouter(orders, drivers)
	for _, role := range []string{"CUSTOMER", "OPERATOR", "ADMIN"} {
		headers := map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)}
		rec := do(h, http.MethodPost, resultPath, resultDeliveredBody, headers)
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
	}
	assert.Nil(t, drivers.registerReq)
	assert.Nil(t, orders.outcomeReq)
}

// Sin fuente de paradas, Driver Service responde UNIMPLEMENTED: el gateway lo traduce a 501 y el pedido
// no se cierra. Es el estado real hasta que exista el origen de paradas (ver el registro de avance).
func TestDriverResult_DriverWithoutStopSourceIsNotImplementedAndOrderUntouched(t *testing.T) {
	orders, drivers := resultFakes()
	drivers.registerErr = status.Error(codes.Unimplemented, "stop source is not available yet")

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Equal(t, "NOT_IMPLEMENTED", decodeError(t, rec).Code)
	assert.Nil(t, orders.outcomeReq)
}

// La ubicación GPS es obligatoria: sin ella Driver Service no puede verificar la llegada.
func TestDriverResult_MissingCoordinatesAreInvalidArgument(t *testing.T) {
	orders, drivers := resultFakes()

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, `{"outcome":"DELIVERED"}`, driverStopHeaders(t))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, drivers.registerReq)
	assert.Nil(t, orders.outcomeReq)
}
