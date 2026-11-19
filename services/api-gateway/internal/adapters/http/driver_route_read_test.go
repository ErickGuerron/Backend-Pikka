package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

const readRouteID = "route-7"

// readStopsOrders registra la petición de paradas y devuelve la respuesta preparada.
type readStopsOrders struct {
	httpapi.OrderClient
	mu    sync.Mutex
	res   *ordersv1.ListDriverRouteStopsResponse
	err   error
	calls int
	got   string
}

func (f *readStopsOrders) ListDriverRouteStops(_ context.Context, _ grpcclient.CallMeta, req *ordersv1.ListDriverRouteStopsRequest) (*ordersv1.ListDriverRouteStopsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.got = req.GetRouteId()
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}

// readRouteDrivers devuelve el repartidor con la ruta indicada, o NOT_FOUND si no hay perfil.
type readRouteDrivers struct {
	httpapi.DriverClient
	routeID   string
	noProfile bool
}

func (f *readRouteDrivers) GetDriverByUserID(_ context.Context, _ grpcclient.CallMeta, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	if f.noProfile {
		return nil, status.Error(codes.NotFound, "driver not found")
	}
	return &driversv1.GetDriverByUserIDResponse{Driver: &driversv1.DriverSummary{Id: "d1", UserId: req.GetUserId(), RouteId: f.routeID}}, nil
}

func readRouteGet(t *testing.T, orders httpapi.OrderClient, drivers httpapi.DriverClient, role string) *httptest.ResponseRecorder {
	t.Helper()
	router := routerFor(trackingOptions(orders, drivers, nil), io.Discard)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/drivers/me/route", nil)
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+token(t, "u-driver", role))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func twoStops() *ordersv1.ListDriverRouteStopsResponse {
	return &ordersv1.ListDriverRouteStopsResponse{Stops: []*ordersv1.DriverRouteStop{
		{OrderId: "o-1", Sequence: 1, Address: "Av. Amazonas 100", WindowStart: "10:00", WindowEnd: "12:00",
			Latitude: -0.18, Longitude: -78.48, Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED},
		{OrderId: "o-2", Sequence: 2, Address: "Calle Larga 5", WindowStart: "12:00", WindowEnd: "14:00",
			Latitude: -0.19, Longitude: -78.49, Status: ordersv1.OrderStatus_ORDER_STATUS_DELIVERED},
	}}
}

type readRouteBody struct {
	ID    string `json:"id"`
	Stops []struct {
		ID          string  `json:"id"`
		Sequence    int     `json:"sequence"`
		Address     string  `json:"address"`
		WindowStart string  `json:"windowStart"`
		WindowEnd   string  `json:"windowEnd"`
		Status      string  `json:"status"`
		Reason      *string `json:"reason"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
	} `json:"stops"`
}

func TestDriverRouteRead_DriverGetsOwnStopsInSequence(t *testing.T) {
	orders := &readStopsOrders{res: twoStops()}
	drivers := &readRouteDrivers{routeID: readRouteID}

	rec := readRouteGet(t, orders, drivers, "DRIVER")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, readRouteID, orders.got, "the stops must be read for the driver's own route")
	var body readRouteBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, readRouteID, body.ID)
	require.Len(t, body.Stops, 2)
	first := body.Stops[0]
	assert.Equal(t, "o-1", first.ID)
	assert.Equal(t, 1, first.Sequence)
	assert.Equal(t, "Av. Amazonas 100", first.Address)
	assert.Equal(t, "10:00", first.WindowStart)
	assert.Equal(t, "12:00", first.WindowEnd)
	assert.Equal(t, "PENDING", first.Status, "ROUTE_ASSIGNED is shown to the driver as PENDING")
	assert.Nil(t, first.Reason)
	assert.InDelta(t, -0.18, first.Latitude, 1e-9)
	assert.Equal(t, "DELIVERED", body.Stops[1].Status)
}

func TestDriverRouteRead_NoRouteIsAnEmptyOK(t *testing.T) {
	orders := &readStopsOrders{res: twoStops()}
	drivers := &readRouteDrivers{routeID: ""}

	rec := readRouteGet(t, orders, drivers, "DRIVER")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"id":"","stops":[]}`, rec.Body.String())
	assert.Equal(t, 0, orders.calls, "without a route the orders service must not be called")
}

func TestDriverRouteRead_NoDriverProfileIsAnEmptyOK(t *testing.T) {
	orders := &readStopsOrders{res: twoStops()}
	drivers := &readRouteDrivers{noProfile: true}

	rec := readRouteGet(t, orders, drivers, "DRIVER")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"id":"","stops":[]}`, rec.Body.String())
	assert.Equal(t, 0, orders.calls)
}

func TestDriverRouteRead_NonDriverRolesAre403(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "ADMIN", "OPERATOR"} {
		orders := &readStopsOrders{res: twoStops()}
		drivers := &readRouteDrivers{routeID: readRouteID}

		rec := readRouteGet(t, orders, drivers, role)

		assert.Equal(t, http.StatusForbidden, rec.Code, "role %s", role)
		assert.Equal(t, 0, orders.calls, "role %s must not read stops", role)
	}
}

func TestDriverRouteRead_WithoutTokenIs401(t *testing.T) {
	orders := &readStopsOrders{res: twoStops()}
	drivers := &readRouteDrivers{routeID: readRouteID}

	rec := readRouteGet(t, orders, drivers, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 0, orders.calls)
}

func TestDriverRouteRead_ResponseNeverExposesCustomerData(t *testing.T) {
	orders := &readStopsOrders{res: twoStops()}
	drivers := &readRouteDrivers{routeID: readRouteID}

	rec := readRouteGet(t, orders, drivers, "DRIVER")

	body := strings.ToLower(rec.Body.String())
	for _, forbidden := range []string{"phone", "customer", "contact", "user_id", "userid"} {
		assert.NotContains(t, body, forbidden)
	}
}

func TestDriverRouteRead_OrdersUnavailableIs503(t *testing.T) {
	orders := &readStopsOrders{err: status.Error(codes.Unavailable, "down")}
	drivers := &readRouteDrivers{routeID: readRouteID}

	rec := readRouteGet(t, orders, drivers, "DRIVER")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
