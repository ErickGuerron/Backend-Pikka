package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

const (
	trackedOrderID  = "7d1c5f0e-0000-4000-8000-000000000001"
	otherOrderID    = "7d1c5f0e-0000-4000-8000-0000000000ff"
	trackedRouteID  = "zone-1#1"
	trackedDriverID = "drv-secret-7"
	// Coordenadas del pedido: no deben aparecer en logs ni en la respuesta.
	trackedLatitude  = -0.1807
	trackedLongitude = -78.4678
)

// trackingOrdersFake sirve las lecturas de pedido del cliente. Embebe la interfaz: cualquier método
// no usado por el test provoca pánico en vez de pasar en silencio. El mutex permite cambiar el
// estado desde el test mientras el handler de WebSocket lo lee en otra goroutine.
type trackingOrdersFake struct {
	httpapi.OrderClient
	mu          sync.Mutex
	order       *ordersv1.Order
	getErr      error
	stopsBefore int32
	stopsErr    error
	advanceRes  *ordersv1.AdvanceRouteStopResponse
	advanceErr  error
	getCalls    int
	stopsCalls  int
	advanceReq  *ordersv1.AdvanceRouteStopRequest
	gotGetID    string
	gotStopsID  string
	gotMeta     grpcclient.CallMeta
}

func (f *trackingOrdersFake) GetMyOrder(_ context.Context, m grpcclient.CallMeta, req *ordersv1.GetMyOrderRequest) (*ordersv1.GetMyOrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	f.gotGetID, f.gotMeta = req.GetOrderId(), m
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &ordersv1.GetMyOrderResponse{Order: f.order}, nil
}

func (f *trackingOrdersFake) StopsBeforeForOrder(_ context.Context, _ grpcclient.CallMeta, req *ordersv1.StopsBeforeForOrderRequest) (*ordersv1.StopsBeforeForOrderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopsCalls++
	f.gotStopsID = req.GetOrderId()
	if f.stopsErr != nil {
		return nil, f.stopsErr
	}
	return &ordersv1.StopsBeforeForOrderResponse{StopsBefore: f.stopsBefore}, nil
}

func (f *trackingOrdersFake) AdvanceRouteStop(_ context.Context, m grpcclient.CallMeta, req *ordersv1.AdvanceRouteStopRequest) (*ordersv1.AdvanceRouteStopResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advanceReq, f.gotMeta = req, m
	if f.advanceErr != nil {
		return nil, f.advanceErr
	}
	return f.advanceRes, nil
}

// setStatus cambia el estado del pedido que devuelve GetMyOrder.
func (f *trackingOrdersFake) setStatus(s ordersv1.OrderStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.order.Status = s
}

func (f *trackingOrdersFake) calls() (get, stops int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getCalls, f.stopsCalls
}

// routeDriversFake resuelve la ruta a un repartidor y el user id a un repartidor.
// byRoute vacío significa que ningún repartidor tiene la ruta (NOT_FOUND).
type routeDriversFake struct {
	httpapi.DriverClient
	byRoute    string
	routeErr   error
	userDriver string
	routeCalls int
	gotRouteID string
}

func (f *routeDriversFake) GetDriverByRouteID(_ context.Context, _ grpcclient.CallMeta, req *driversv1.GetDriverByRouteIDRequest) (*driversv1.GetDriverByRouteIDResponse, error) {
	f.routeCalls++
	f.gotRouteID = req.GetRouteId()
	if f.routeErr != nil {
		return nil, f.routeErr
	}
	if f.byRoute == "" {
		return nil, status.Error(codes.NotFound, "no driver holds this route")
	}
	return &driversv1.GetDriverByRouteIDResponse{DriverId: f.byRoute, UserId: "u-driver"}, nil
}

func (f *routeDriversFake) GetDriverByUserID(_ context.Context, _ grpcclient.CallMeta, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	if f.userDriver == "" {
		return nil, status.Error(codes.NotFound, "no driver for this user")
	}
	return &driversv1.GetDriverByUserIDResponse{Driver: &driversv1.DriverSummary{Id: f.userDriver, UserId: req.GetUserId()}}, nil
}

// trackingETAFake responde EstimateArrival y registra a qué repartidor y destino se pidió.
type trackingETAFake struct {
	httpapi.TrackingClient
	res       *trackingv1.EstimateArrivalResponse
	err       error
	calls     int
	gotDriver string
	gotReq    *trackingv1.EstimateArrivalRequest
}

func (f *trackingETAFake) EstimateArrival(_ context.Context, _ grpcclient.CallMeta, req *trackingv1.EstimateArrivalRequest) (*trackingv1.EstimateArrivalResponse, error) {
	f.calls++
	f.gotDriver, f.gotReq = req.GetDriverId(), req
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}

// GetDriverPosition: estos tests se centran en la estimación; sin posición fresca el snapshot devuelve null.
func (f *trackingETAFake) GetDriverPosition(_ context.Context, _ grpcclient.CallMeta, _ *trackingv1.GetDriverPositionRequest) (*trackingv1.GetDriverPositionResponse, error) {
	return nil, status.Error(codes.NotFound, "no fresh position")
}

// ownOrder es un pedido del cliente con ruta asignada.
func ownOrder(s ordersv1.OrderStatus, routeID string) *ordersv1.Order {
	return &ordersv1.Order{
		Id: trackedOrderID, Address: "Av. Amazonas 123", Latitude: trackedLatitude, Longitude: trackedLongitude,
		DeliveryDate: "2026-10-20", WindowStart: "10:00", WindowEnd: "12:00", Status: s,
		CreatedAt: "2026-10-16T12:00:00Z", RouteId: routeID,
	}
}

func trackingOptions(orders httpapi.OrderClient, drivers httpapi.DriverClient, tracking httpapi.TrackingClient) httpapi.Options {
	return httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		RequestTimeout: 5 * time.Second, BodyLimit: "1K", EnableHSTS: true,
		Orders: orders, Drivers: drivers, Tracking: tracking,
	}
}

func routerFor(opts httpapi.Options, out io.Writer) http.Handler {
	log := slog.New(slog.NewJSONHandler(out, nil))
	return httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, func(context.Context) bool { return true })
}

func customerHeaders(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": "Bearer " + token(t, "u-customer", "CUSTOMER")}
}

func snapshotPath(orderID string) string {
	return "/api/v1/orders/" + orderID + "/tracking"
}

type etaBody struct {
	DistanceMeters  float64 `json:"distance_meters"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type windowBody struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type snapshotBody struct {
	OrderID              string     `json:"order_id"`
	Status               string     `json:"status"`
	DeliveryAddress      string     `json:"delivery_address"`
	DeliveryWindow       windowBody `json:"delivery_window"`
	ETA                  *etaBody   `json:"eta"`
	ETAUnavailableReason *string    `json:"eta_unavailable_reason"`
	StopsBefore          *int       `json:"stops_before"`
	UpdatedAt            string     `json:"updated_at"`
}

func decodeSnapshot(t *testing.T, rec *httptest.ResponseRecorder) snapshotBody {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body snapshotBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func intPtr(n int) *int { return &n }

func TestTrackingSnapshot_NonCustomerRolesGet403BeforeAnyCall(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{}), io.Discard)

	for _, role := range []string{"DRIVER", "OPERATOR", "ADMIN"} {
		headers := map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)}
		rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", headers)
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
	}
	get, _ := orders.calls()
	assert.Zero(t, get)
}

func TestTrackingSnapshot_WithoutToken_Returns401(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	h := routerFor(trackingOptions(orders, &routeDriversFake{}, &trackingETAFake{}), io.Discard)

	rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestTrackingSnapshot_OwnOrderWithFreshPosition_ReturnsETAAndStopsAhead(t *testing.T) {
	orders := &trackingOrdersFake{
		order:       ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID),
		stopsBefore: 1,
	}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	eta := &trackingETAFake{res: &trackingv1.EstimateArrivalResponse{DistanceMeters: 1234.5, DurationSeconds: 240}}
	h := routerFor(trackingOptions(orders, drivers, eta), io.Discard)

	rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t))

	body := decodeSnapshot(t, rec)
	assert.Equal(t, trackedOrderID, body.OrderID)
	assert.Equal(t, "ROUTE_ASSIGNED", body.Status)
	assert.Equal(t, "Av. Amazonas 123", body.DeliveryAddress)
	assert.Equal(t, windowBody{Start: "10:00", End: "12:00"}, body.DeliveryWindow)
	require.NotNil(t, body.ETA)
	assert.Equal(t, 1234.5, body.ETA.DistanceMeters)
	assert.Equal(t, 240.0, body.ETA.DurationSeconds)
	assert.Nil(t, body.ETAUnavailableReason)
	require.NotNil(t, body.StopsBefore)
	assert.Equal(t, 1, *body.StopsBefore, "the count comes from Order Service")
	_, err := time.Parse(time.RFC3339, body.UpdatedAt)
	assert.NoError(t, err, "updated_at is RFC 3339")
	assert.Equal(t, trackedOrderID, orders.gotStopsID, "stops_before is asked for the customer's own order only")
	assert.Equal(t, trackedRouteID, drivers.gotRouteID)
	assert.Equal(t, trackedDriverID, eta.gotDriver, "the driver id only goes to Tracking")
	assert.InDelta(t, trackedLatitude, eta.gotReq.GetDestinationLatitude(), 1e-9)
	assert.InDelta(t, trackedLongitude, eta.gotReq.GetDestinationLongitude(), 1e-9)
}

// stops_before se copia tal cual del servicio. Sin ruta no se pregunta; si el servicio falla, es desconocido.
func TestTrackingSnapshot_StopsBeforeIsTheServiceCount(t *testing.T) {
	cases := []struct {
		name   string
		count  int32
		err    error
		routed bool
		want   *int
	}{
		{"nothing ahead", 0, nil, true, intPtr(0)},
		{"open stops ahead", 2, nil, true, intPtr(2)},
		{"service says the order is not visible", 0, status.Error(codes.NotFound, "pedido no encontrado"), true, nil},
		{"service unavailable", 0, status.Error(codes.Unavailable, "down"), true, nil},
		{"no route yet", 0, nil, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routeID := trackedRouteID
			if !tc.routed {
				routeID = ""
			}
			orders := &trackingOrdersFake{
				order:       ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, routeID),
				stopsBefore: tc.count,
				stopsErr:    tc.err,
			}
			eta := &trackingETAFake{res: &trackingv1.EstimateArrivalResponse{DistanceMeters: 10, DurationSeconds: 1}}
			h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, eta), io.Discard)

			body := decodeSnapshot(t, do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t)))

			assert.Equal(t, tc.want, body.StopsBefore)
			if !tc.routed {
				_, stops := orders.calls()
				assert.Zero(t, stops, "an order without route never asks for stops")
			}
		})
	}
}

// Una orden ajena y una inexistente deben responder igual: el cliente no puede saber cuál es cuál.
func TestTrackingSnapshot_ForeignAndMissingOrderReturnTheSame404(t *testing.T) {
	orders := &trackingOrdersFake{getErr: status.Error(codes.NotFound, "pedido no encontrado")}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	eta := &trackingETAFake{}
	h := routerFor(trackingOptions(orders, drivers, eta), io.Discard)

	foreign := do(h, http.MethodGet, snapshotPath(otherOrderID), "", customerHeaders(t))
	missing := do(h, http.MethodGet, snapshotPath("7d1c5f0e-0000-4000-8000-0000000000aa"), "", customerHeaders(t))

	require.Equal(t, http.StatusNotFound, foreign.Code)
	require.Equal(t, http.StatusNotFound, missing.Code)
	assert.Equal(t, decodeError(t, foreign).Code, decodeError(t, missing).Code)
	assert.Equal(t, decodeError(t, foreign).Message, decodeError(t, missing).Message)
	assert.Zero(t, drivers.routeCalls)
	assert.Zero(t, eta.calls)
}

func TestTrackingSnapshot_OrderWithoutRouteHasNullETAAndNoRouteReason(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_PENDING, "")}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	eta := &trackingETAFake{}
	h := routerFor(trackingOptions(orders, drivers, eta), io.Discard)

	rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t))

	body := decodeSnapshot(t, rec)
	assert.Nil(t, body.ETA)
	require.NotNil(t, body.ETAUnavailableReason)
	assert.Equal(t, "no_route", *body.ETAUnavailableReason)
	assert.Nil(t, body.StopsBefore)
	assert.Zero(t, drivers.routeCalls)
	assert.Zero(t, orders.stopsCalls)
	assert.Zero(t, eta.calls)
	assert.Contains(t, rec.Body.String(), `"eta":null`)
}

func TestTrackingSnapshot_RouteWithoutDriverHasNoRouteReason(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{byRoute: ""}
	eta := &trackingETAFake{}
	h := routerFor(trackingOptions(orders, drivers, eta), io.Discard)

	body := decodeSnapshot(t, do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t)))

	assert.Nil(t, body.ETA)
	require.NotNil(t, body.ETAUnavailableReason)
	assert.Equal(t, "no_route", *body.ETAUnavailableReason)
	assert.Zero(t, eta.calls)
}

// Tracking responde por código gRPC. Nunca se inventa un ETA; el snapshot sigue siendo 200.
func TestTrackingSnapshot_TrackingErrorsMapToReasonWithoutFakeETA(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		reason string
	}{
		{"no position", status.Error(codes.NotFound, "no position"), "no_position"},
		{"stale position", status.Error(codes.FailedPrecondition, "stale"), "stale_position"},
		{"internal", status.Error(codes.Internal, "routing down"), "unavailable"},
		{"unavailable", status.Error(codes.Unavailable, "down"), "unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orders := &trackingOrdersFake{
				order:       ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID),
				stopsBefore: 0,
			}
			eta := &trackingETAFake{err: tc.err}
			h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, eta), io.Discard)

			body := decodeSnapshot(t, do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t)))

			assert.Nil(t, body.ETA)
			require.NotNil(t, body.ETAUnavailableReason)
			assert.Equal(t, tc.reason, *body.ETAUnavailableReason)
			assert.Equal(t, intPtr(0), body.StopsBefore, "the rest of the snapshot is still served")
		})
	}
}

func TestTrackingSnapshot_DriverLookupInternalErrorIsUnavailable(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{routeErr: status.Error(codes.Internal, "boom")}
	eta := &trackingETAFake{}
	h := routerFor(trackingOptions(orders, drivers, eta), io.Discard)

	body := decodeSnapshot(t, do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t)))

	assert.Nil(t, body.ETA)
	require.NotNil(t, body.ETAUnavailableReason)
	assert.Equal(t, "unavailable", *body.ETAUnavailableReason)
	assert.Zero(t, eta.calls)
}

func TestTrackingSnapshot_StopsBeforeFailureKeepsETA(t *testing.T) {
	orders := &trackingOrdersFake{
		order:    ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID),
		stopsErr: status.Error(codes.Internal, "boom"),
	}
	eta := &trackingETAFake{res: &trackingv1.EstimateArrivalResponse{DistanceMeters: 50, DurationSeconds: 9}}
	h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, eta), io.Discard)

	body := decodeSnapshot(t, do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t)))

	assert.Nil(t, body.StopsBefore)
	require.NotNil(t, body.ETA)
	assert.Equal(t, 50.0, body.ETA.DistanceMeters)
}

// La respuesta del cliente no lleva identidad del repartidor, ruta, otros pedidos ni campos de posición.
func TestTrackingSnapshot_ResponseLeaksNoDriverRouteOrOtherOrders(t *testing.T) {
	orders := &trackingOrdersFake{
		order:       ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID),
		stopsBefore: 1,
	}
	eta := &trackingETAFake{res: &trackingv1.EstimateArrivalResponse{DistanceMeters: 1, DurationSeconds: 2}}
	h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID, userDriver: trackedDriverID}, eta), io.Discard)

	raw := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t)).Body.String()

	// "driver_position" is the position of the driver, not an identity: only identifiers must stay out.
	for _, leak := range []string{trackedDriverID, trackedRouteID, otherOrderID, "u-driver", "driver_id", "driverId", "route"} {
		assert.NotContains(t, raw, leak)
	}
}

// Los logs no llevan token, coordenadas ni id del repartidor, ni en éxito ni en error.
func TestTrackingSnapshot_LogsNeverContainTokenCoordinatesOrDriverID(t *testing.T) {
	var out bytes.Buffer
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	eta := &trackingETAFake{err: status.Error(codes.Internal, "boom")}
	h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, eta), &out)
	bearer := token(t, "u-customer", "CUSTOMER")

	do(h, http.MethodGet, snapshotPath(trackedOrderID), "", map[string]string{"Authorization": "Bearer " + bearer})
	orders.getErr = status.Error(codes.NotFound, "pedido no encontrado")
	do(h, http.MethodGet, snapshotPath(otherOrderID), "", map[string]string{"Authorization": "Bearer " + bearer})

	logs := out.String()
	require.NotEmpty(t, logs, "the request logger wrote something")
	for _, secret := range []string{bearer, trackedDriverID, "0.1807", "78.4678"} {
		assert.False(t, strings.Contains(logs, secret), "log leaks %q", secret)
	}
}
