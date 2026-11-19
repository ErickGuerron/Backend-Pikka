package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// syncBuffer deja leer los logs que escribe la goroutine del servidor sin carrera de datos.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// fakeTicker es el reloj de sondeo del test: cada tick lo manda el test, así no hay sleeps.
type fakeTicker struct {
	t       *testing.T
	ch      chan time.Time
	started chan time.Duration
}

func newFakeTicker(t *testing.T) *fakeTicker {
	return &fakeTicker{t: t, ch: make(chan time.Time), started: make(chan time.Duration, 1)}
}

// ticker es la función que recibe Options.TrackingTicker.
func (f *fakeTicker) ticker(interval time.Duration) (<-chan time.Time, func()) {
	f.started <- interval
	return f.ch, func() {}
}

// tick entrega un sondeo y espera a que el handler lo tome. Como el handler procesa los sondeos en
// orden, el siguiente tick solo se acepta cuando el anterior ya terminó.
func (f *fakeTicker) tick() {
	f.t.Helper()
	select {
	case f.ch <- time.Now():
	case <-time.After(5 * time.Second):
		f.t.Fatal("the handler did not take the poll tick")
	}
}

func trackingWSServer(t *testing.T, orders httpapi.OrderClient, drivers httpapi.DriverClient, eta httpapi.TrackingClient, ft *fakeTicker, logs io.Writer) *httptest.Server {
	t.Helper()
	opts := trackingOptions(orders, drivers, eta)
	opts.TrackingTicker = ft.ticker
	srv := httptest.NewServer(routerFor(opts, logs))
	t.Cleanup(srv.Close)
	return srv
}

func trackingWSURL(srv *httptest.Server, orderID string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + snapshotPath(orderID) + "/ws"
}

func readSnapshotMessage(t *testing.T, conn *websocket.Conn) snapshotBody {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	var body snapshotBody
	require.NoError(t, json.Unmarshal(data, &body))
	return body
}

// dialTracking abre el socket con el protocolo pikka.v1 y el token como segundo subprotocolo.
func dialTracking(t *testing.T, srv *httptest.Server, orderID string, subprotocols ...string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return websocket.Dial(ctx, trackingWSURL(srv, orderID), &websocket.DialOptions{Subprotocols: subprotocols})
}

func TestTrackingWS_RejectsMissingOrInvalidCredentialsBeforeUpgrade(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	srv := trackingWSServer(t, orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{}, newFakeTicker(t), io.Discard)
	driverTok := token(t, "u-driver", "DRIVER")

	cases := []struct {
		name         string
		subprotocols []string
		wantStatus   int
	}{
		{"no token", []string{"pikka.v1"}, http.StatusUnauthorized},
		{"invalid token", []string{"pikka.v1", "not-a-jwt"}, http.StatusUnauthorized},
		{"token in the wrong position", []string{"not-pikka", token(t, "u-customer", "CUSTOMER")}, http.StatusUnauthorized},
		{"driver role", []string{"pikka.v1", driverTok}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn, resp, err := dialTracking(t, srv, trackedOrderID, tc.subprotocols...)
			if resp != nil && resp.Body != nil {
				defer func() { _ = resp.Body.Close() }()
			}

			require.Error(t, err)
			assert.Nil(t, conn)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
	get, _ := orders.calls()
	assert.Zero(t, get, "a rejected socket never reaches the order service")
}

func TestTrackingWS_ForeignOrderIsNotFoundBeforeUpgrade(t *testing.T) {
	orders := &trackingOrdersFake{getErr: status.Error(codes.NotFound, "pedido no encontrado")}
	srv := trackingWSServer(t, orders, &routeDriversFake{}, &trackingETAFake{}, newFakeTicker(t), io.Discard)

	_, resp, err := dialTracking(t, srv, otherOrderID, "pikka.v1", token(t, "u-customer", "CUSTOMER"))
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTrackingWS_AcceptsCustomerAndSendsSnapshotOnConnect(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	ft := newFakeTicker(t)
	srv := trackingWSServer(t, orders, &routeDriversFake{byRoute: trackedDriverID},
		&trackingETAFake{res: &trackingv1.EstimateArrivalResponse{DistanceMeters: 800, DurationSeconds: 90}}, ft, io.Discard)

	conn, resp, err := dialTracking(t, srv, trackedOrderID, "pikka.v1", token(t, "u-customer", "CUSTOMER"))
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.CloseNow() }()

	assert.Equal(t, "pikka.v1", conn.Subprotocol(), "the server echoes the protocol, never the token")
	first := readSnapshotMessage(t, conn)
	assert.Equal(t, trackedOrderID, first.OrderID)
	assert.Equal(t, "ROUTE_ASSIGNED", first.Status)
	require.NotNil(t, first.ETA)
	assert.Equal(t, 90.0, first.ETA.DurationSeconds)
	assert.Equal(t, 5*time.Second, <-ft.started, "the default poll interval is 5 s")
}

// Solo se envía un mensaje cuando el snapshot cambia. Los sondeos sin cambio no producen salida.
// Los ticks son síncronos, así que el orden de los mensajes es determinista y no hay sleeps.
func TestTrackingWS_PushesOnlyWhenSnapshotChanges(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	ft := newFakeTicker(t)
	srv := trackingWSServer(t, orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{}, ft, io.Discard)
	conn, resp, err := dialTracking(t, srv, trackedOrderID, "pikka.v1", token(t, "u-customer", "CUSTOMER"))
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.CloseNow() }()

	require.Equal(t, "ROUTE_ASSIGNED", readSnapshotMessage(t, conn).Status)
	ft.tick() // sin cambio
	ft.tick() // sin cambio; el anterior ya terminó
	orders.setStatus(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)
	ft.tick() // cambio: se envía
	assert.Equal(t, "PICKED_UP", readSnapshotMessage(t, conn).Status)

	ft.tick() // sin cambio
	ft.tick() // sin cambio; el anterior ya terminó
	orders.setStatus(ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT)
	ft.tick() // cambio: se envía
	// Si los sondeos sin cambio hubieran enviado un duplicado de PICKED_UP, aparecería aquí primero.
	assert.Equal(t, "IN_TRANSIT", readSnapshotMessage(t, conn).Status)
}

// El timeout global de petición (RequestTimeout) no debe cortar un socket vivo: un seguimiento dura
// más que cualquier petición. Con un timeout de 50 ms, el socket sigue entregando cambios tras 150 ms.
func TestTrackingWS_SurvivesRequestTimeout(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	ft := newFakeTicker(t)
	opts := trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{})
	opts.RequestTimeout = 50 * time.Millisecond
	opts.TrackingTicker = ft.ticker
	srv := httptest.NewServer(routerFor(opts, io.Discard))
	t.Cleanup(srv.Close)
	conn, resp, err := dialTracking(t, srv, trackedOrderID, "pikka.v1", token(t, "u-customer", "CUSTOMER"))
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.CloseNow() }()
	require.Equal(t, "ROUTE_ASSIGNED", readSnapshotMessage(t, conn).Status)

	time.Sleep(150 * time.Millisecond) // más que RequestTimeout
	orders.setStatus(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)
	ft.tick()

	assert.Equal(t, "PICKED_UP", readSnapshotMessage(t, conn).Status)
}

func TestTrackingWS_LogsNeverContainTokenOrCoordinates(t *testing.T) {
	var logs syncBuffer
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	ft := newFakeTicker(t)
	srv := trackingWSServer(t, orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{}, ft, &logs)
	tok := token(t, "u-customer", "CUSTOMER")
	conn, resp, err := dialTracking(t, srv, trackedOrderID, "pikka.v1", tok)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	readSnapshotMessage(t, conn)
	require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))

	// El log de la petición se escribe al cerrar el socket; sondea hasta verlo sin sleep fijo.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "http request") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	out := logs.String()
	require.Contains(t, out, "http request")
	for _, secret := range []string{tok, trackedDriverID, "0.1807", "78.4678"} {
		assert.False(t, strings.Contains(out, secret), "log leaks %q", secret)
	}
}
