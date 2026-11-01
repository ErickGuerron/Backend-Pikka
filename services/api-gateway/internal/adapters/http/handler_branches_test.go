package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

// stopDrivers responde las búsquedas de repartidor de las rutas de parada. Con failUserAt, la búsqueda
// por usuario número N devuelve userErr; así se recorre cada paso de la secuencia del gateway.
type stopDrivers struct {
	httpapi.DriverClient
	driverID   string
	routeID    string
	userCalls  int
	failUserAt int
	userErr    error
	routeErr   error
	startErr   error
}

func (f *stopDrivers) GetDriverByUserID(context.Context, grpcclient.CallMeta, *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	f.userCalls++
	if f.failUserAt == f.userCalls {
		return nil, f.userErr
	}
	return &driversv1.GetDriverByUserIDResponse{Driver: &driversv1.DriverSummary{Id: f.driverID, RouteId: f.routeID}}, nil
}

func (f *stopDrivers) GetDriverByRouteID(context.Context, grpcclient.CallMeta, *driversv1.GetDriverByRouteIDRequest) (*driversv1.GetDriverByRouteIDResponse, error) {
	if f.routeErr != nil {
		return nil, f.routeErr
	}
	return &driversv1.GetDriverByRouteIDResponse{DriverId: f.driverID, UserId: "u-driver"}, nil
}

func (f *stopDrivers) StartRoute(context.Context, grpcclient.CallMeta, *driversv1.StartRouteRequest) (*driversv1.StartRouteResponse, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &driversv1.StartRouteResponse{}, nil
}

func driverHeader(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": "Bearer " + token(t, "u-driver", "DRIVER")}
}

// --- Parada: PATCH /drivers/me/routes/:routeId/stops/:stopId/status ---------

func TestDriverStop_UpstreamFailuresAreReportedNotSwallowed(t *testing.T) {
	cases := []struct {
		name    string
		drivers *stopDrivers
		target  string
	}{
		{"búsqueda de repartidor por usuario", &stopDrivers{driverID: "drv-1", failUserAt: 1, userErr: upstreamDown}, "IN_TRANSIT"},
		{"búsqueda de ruta del repartidor", &stopDrivers{driverID: "drv-1", routeErr: upstreamDown}, "IN_TRANSIT"},
		{"segunda búsqueda de repartidor al iniciar la ruta", &stopDrivers{driverID: "drv-1", failUserAt: 2, userErr: upstreamDown}, "PICKED_UP"},
		{"inicio de ruta en Driver Service", &stopDrivers{driverID: "drv-1", startErr: upstreamDown}, "PICKED_UP"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := routerFor(trackingOptions(&trackingOrdersFake{}, tc.drivers, nil), io.Discard)
			path := "/api/v1/drivers/me/routes/r-1/stops/" + trackedOrderID + "/status"
			rec := do(h, http.MethodPatch, path, `{"status":"`+tc.target+`"}`, driverHeader(t))
			assertUpstreamDown(t, rec)
		})
	}
}

// --- Resultado de parada: POST /drivers/me/routes/:routeId/stops/:stopId/result ----

func TestDriverResult_UpstreamFailuresAreReportedNotSwallowed(t *testing.T) {
	cases := []struct {
		name    string
		orders  *trackingOrdersFake
		drivers *stopDrivers
	}{
		{"búsqueda de repartidor por usuario",
			&trackingOrdersFake{}, &stopDrivers{driverID: "drv-1", failUserAt: 1, userErr: upstreamDown}},
		{"lista de paradas de la ruta",
			&trackingOrdersFake{routeStopsErr: upstreamDown}, &stopDrivers{driverID: "drv-1"}},
		{"segunda búsqueda de repartidor al registrar la entrega",
			&trackingOrdersFake{routeStopsRes: stopsWith(ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT)},
			&stopDrivers{driverID: "drv-1", failUserAt: 2, userErr: upstreamDown}},
	}
	body := `{"outcome":"DELIVERED","latitude":-0.18,"longitude":-78.48}`
	path := "/api/v1/drivers/me/routes/r-1/stops/" + trackedOrderID + "/result"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := routerFor(trackingOptions(tc.orders, tc.drivers, nil), io.Discard)
			assertUpstreamDown(t, do(h, http.MethodPost, path, body, driverHeader(t)))
		})
	}
}

// --- Ruta del repartidor: GET /drivers/me/route -----------------------------

func TestDriverRoute_LookupFailureAndEveryStopStatusHasItsView(t *testing.T) {
	t.Run("búsqueda de repartidor propaga el fallo", func(t *testing.T) {
		drivers := &stopDrivers{driverID: "drv-1", routeID: "r-1", failUserAt: 1, userErr: upstreamDown}
		h := routerFor(trackingOptions(&trackingOrdersFake{}, drivers, nil), io.Discard)
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/drivers/me/route", "", driverHeader(t)))
	})

	t.Run("cada estado de parada se muestra con su etiqueta", func(t *testing.T) {
		orders := &trackingOrdersFake{routeStopsRes: &ordersv1.ListDriverRouteStopsResponse{Stops: []*ordersv1.DriverRouteStop{
			{OrderId: "o-1", Sequence: 1, Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED},
			{OrderId: "o-2", Sequence: 2, Status: ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT},
			{OrderId: "o-3", Sequence: 3, Status: ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED},
			{OrderId: "o-4", Sequence: 4, Status: ordersv1.OrderStatus_ORDER_STATUS_DELIVERED},
			{OrderId: "o-5", Sequence: 5, Status: ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP},
		}}}
		drivers := &stopDrivers{driverID: "drv-1", routeID: "r-1"}
		h := routerFor(trackingOptions(orders, drivers, nil), io.Discard)

		rec := do(h, http.MethodGet, "/api/v1/drivers/me/route", "", driverHeader(t))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body struct {
			Stops []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"stops"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		got := map[string]string{}
		for _, s := range body.Stops {
			got[s.ID] = s.Status
		}
		assert.Equal(t, map[string]string{
			"o-1": "PENDING", "o-2": "IN_TRANSIT", "o-3": "NOT_DELIVERED", "o-4": "DELIVERED", "o-5": "PICKED_UP",
		}, got)
	})
}

// --- Plan de ruta: GET /drivers/me/route/plan -------------------------------

func TestDriverRoutePlan_UpstreamFailuresAndRouteWithoutId(t *testing.T) {
	t.Run("búsqueda de repartidor propaga el fallo", func(t *testing.T) {
		drivers := &stopDrivers{driverID: "drv-1", routeID: "r-1", failUserAt: 1, userErr: upstreamDown}
		rec := planGet(t, trackingOptions(&trackingOrdersFake{}, drivers, nil), "DRIVER", "?lat=-0.18&lon=-78.48")
		assertUpstreamDown(t, rec)
	})
	t.Run("lista de paradas propaga el fallo", func(t *testing.T) {
		drivers := &stopDrivers{driverID: "drv-1", routeID: "r-1"}
		orders := &trackingOrdersFake{routeStopsErr: upstreamDown}
		rec := planGet(t, trackingOptions(orders, drivers, nil), "DRIVER", "?lat=-0.18&lon=-78.48")
		assertUpstreamDown(t, rec)
	})
	t.Run("repartidor sin id de ruta recibe un plan vacío", func(t *testing.T) {
		drivers := &stopDrivers{driverID: "drv-1"}
		rec := planGet(t, trackingOptions(&trackingOrdersFake{}, drivers, nil), "DRIVER", "?lat=-0.18&lon=-78.48")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `{"order":[],"distanceMeters":0,"durationSeconds":0,"geometry":[]}`, rec.Body.String())
	})
}

// --- Repartidores: alta y listado con sus mapeos de vehículo y estado ---------

func TestCreateDriver_VehicleTypesRoundTrip(t *testing.T) {
	for _, vehicle := range []string{"AUTOMOVIL", "SUV", "MOTORIZADO"} {
		t.Run(vehicle, func(t *testing.T) {
			body := strings.Replace(createDriverBody, `"MOTORIZADO"`, `"`+vehicle+`"`, 1)
			rec := do(createDriverRouter(&fakeAuth{}, &creatingDrivers{}), http.MethodPost, "/api/v1/drivers", body, adminHeader(t))
			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"vehicleType":"`+vehicle+`"`)
		})
	}
}

// statusFleet devuelve una flota con cada estado y tipo de vehículo que el listado debe traducir.
type statusFleet struct{ fakeDriverClient }

func (f *statusFleet) ListDrivers(context.Context, grpcclient.CallMeta, *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	return &driversv1.ListDriversResponse{Drivers: []*driversv1.DriverListItem{
		{Id: "d-1", FirstName: "Ana", LastName: "Ruiz", VehicleType: driversv1.VehicleType_VEHICLE_TYPE_CAR,
			Status: driversv1.DriverStatus_DRIVER_STATUS_ASSIGNED, Active: true},
		{Id: "d-2", FirstName: "Luis", LastName: "Paz", Status: driversv1.DriverStatus_DRIVER_STATUS_ON_ROUTE},
		{Id: "d-3", FirstName: "Eva", LastName: "Sol"},
	}}, nil
}

func TestListDrivers_TranslatesVehicleAndStatus(t *testing.T) {
	h := driversRouter(&statusFleet{})
	rec := do(h, http.MethodGet, "/api/v1/drivers", "", authHdr(t, "OPERATOR"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body []struct {
		ID          string `json:"id"`
		VehicleType string `json:"vehicleType"`
		Status      string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body, 3)
	assert.Equal(t, "AUTOMOVIL", body[0].VehicleType)
	assert.Equal(t, "ASSIGNED", body[0].Status)
	assert.Equal(t, "", body[1].VehicleType, "vehículo sin especificar se muestra vacío")
	assert.Equal(t, "ON_ROUTE", body[1].Status)
	assert.Equal(t, "", body[2].Status, "estado sin mapeo se muestra vacío")
}

// --- Seguimiento: handshake WebSocket y snapshot -----------------------------

// positionOutageTracking falla al leer la posición fresca; la estimación sigue sana.
type positionOutageTracking struct{ trackingETAFake }

func (f *positionOutageTracking) GetDriverPosition(context.Context, grpcclient.CallMeta, *trackingv1.GetDriverPositionRequest) (*trackingv1.GetDriverPositionResponse, error) {
	return nil, upstreamDown
}

func TestTrackingWS_HandshakeFailuresDoNotOpenTheSocket(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	h := routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{}), io.Discard)
	path := snapshotPath(trackedOrderID) + "/ws"

	t.Run("subprotocolo con token vacío es 401", func(t *testing.T) {
		rec := do(h, http.MethodGet, path, "", map[string]string{"Sec-WebSocket-Protocol": "pikka.v1,"})
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("petición sin cabeceras de upgrade no se actualiza", func(t *testing.T) {
		rec := do(h, http.MethodGet, path, "", map[string]string{
			"Sec-WebSocket-Protocol": "pikka.v1, " + token(t, "u-customer", "CUSTOMER"),
		})
		assert.NotEqual(t, http.StatusSwitchingProtocols, rec.Code)
		assert.Equal(t, http.StatusUpgradeRequired, rec.Code, "la librería WebSocket rechaza la petición antes de hijack")
	})
}

func TestTrackingSnapshot_DriverPositionOutageHidesThePosition(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	drivers := &routeDriversFake{byRoute: trackedDriverID}
	h := routerFor(trackingOptions(orders, drivers, &positionOutageTracking{}), io.Discard)

	rec := do(h, http.MethodGet, snapshotPath(trackedOrderID), "", customerHeaders(t))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Nil(t, body["driver_position"], "sin posición fresca no se muestra ubicación del repartidor")
	assert.Equal(t, "ROUTE_ASSIGNED", body["status"])
}

// Sin TrackingTicker inyectado, el socket usa el reloj real y entrega el snapshot inicial.
func TestTrackingWS_DefaultPollingTickerDeliversTheFirstSnapshot(t *testing.T) {
	orders := &trackingOrdersFake{order: ownOrder(ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED, trackedRouteID)}
	srv := httptest.NewServer(routerFor(trackingOptions(orders, &routeDriversFake{byRoute: trackedDriverID}, &trackingETAFake{}), io.Discard))
	t.Cleanup(srv.Close)

	conn, resp, err := dialTracking(t, srv, trackedOrderID, "pikka.v1", token(t, "u-customer", "CUSTOMER"))
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.CloseNow() }()
	assert.Equal(t, "ROUTE_ASSIGNED", readSnapshotMessage(t, conn).Status)
}

// --- Middlewares de rol sin identidad ---------------------------------------

// Sin identidad (que solo deja Authenticate) cada middleware de rol responde 401 y no llama al handler.
func TestRoleMiddlewares_RejectRequestsWithoutIdentity(t *testing.T) {
	cases := map[string]echo.MiddlewareFunc{
		"AdminOnly":    httpapi.AdminOnly(),
		"DriverOnly":   httpapi.DriverOnly(),
		"StaffOnly":    httpapi.StaffOnly(),
		"CustomerOnly": httpapi.CustomerOnly(),
	}
	for name, mw := range cases {
		t.Run(name, func(t *testing.T) {
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			reached := false
			next := func(echo.Context) error {
				reached = true
				return nil
			}

			err := mw(next)(c)

			var he *echo.HTTPError
			require.ErrorAs(t, err, &he)
			assert.Equal(t, http.StatusUnauthorized, he.Code)
			assert.False(t, reached)
		})
	}
}

// --- Manejo de errores HTTP ---------------------------------------------------

func TestErrorHandler_CommittedAndHeadResponses(t *testing.T) {
	t.Run("una respuesta ya enviada no se reescribe", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.Response().WriteHeader(http.StatusOK)

		httpapi.ErrorHandler(echo.ErrNotFound, c)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Body.String())
	})
	t.Run("HEAD responde solo el estado, sin cuerpo", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodHead, "/", nil), rec)

		httpapi.ErrorHandler(echo.ErrNotFound, c)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Empty(t, rec.Body.String())
	})
}

// Con TrustProxy el límite de login se aplica por el cliente real del header X-Forwarded-For, pero solo
// cuando la petición llega desde un proxy de confianza (loopback por defecto de Echo).
func TestLoginRateLimitUsesForwardedClientWhenTrustProxy(t *testing.T) {
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 2,
		RequestTimeout: time.Second, BodyLimit: "1K", TrustProxy: true,
	}
	h := httpapi.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, func(context.Context) bool { return true })
	login := func(forwardedFor string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@example.com","password":"password123"}`))
		req.RemoteAddr = "127.0.0.1:50000"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", forwardedFor)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, login("203.0.113.1"))
	assert.Equal(t, http.StatusTooManyRequests, login("203.0.113.1"))
	assert.Equal(t, http.StatusOK, login("203.0.113.2"), "otro cliente tiene su propio cupo")
}
