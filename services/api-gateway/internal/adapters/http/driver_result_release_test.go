package http_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// releasingDrivers agrega ReleaseDriver al doble de Driver Service, como el cliente gRPC real.
type releasingDrivers struct {
	*routeDriversFake
	released []*driversv1.ReleaseDriverRequest
}

func (f *releasingDrivers) ReleaseDriver(_ context.Context, _ grpcclient.CallMeta, req *driversv1.ReleaseDriverRequest) (*driversv1.ReleaseDriverResponse, error) {
	f.released = append(f.released, req)
	return &driversv1.ReleaseDriverResponse{}, nil
}

// closingOrders devuelve la parada ya cerrada después de registrar su resultado, más las paradas extra.
type closingOrders struct {
	*trackingOrdersFake
	others []*ordersv1.DriverRouteStop
}

func (f *closingOrders) ListDriverRouteStops(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListDriverRouteStopsRequest) (*ordersv1.ListDriverRouteStopsResponse, error) {
	if f.outcomeReq == nil {
		res, err := f.trackingOrdersFake.ListDriverRouteStops(ctx, m, req)
		if err != nil {
			return nil, err
		}
		return &ordersv1.ListDriverRouteStopsResponse{Stops: append(res.GetStops(), f.others...)}, nil
	}
	closed := &ordersv1.DriverRouteStop{OrderId: trackedOrderID, Status: f.outcomeReq.GetOutcome()}
	return &ordersv1.ListDriverRouteStopsResponse{Stops: append([]*ordersv1.DriverRouteStop{closed}, f.others...)}, nil
}

func resultWithRelease(others ...*ordersv1.DriverRouteStop) (*closingOrders, *releasingDrivers, http.Handler) {
	orders, drivers := resultFakes()
	co := &closingOrders{trackingOrdersFake: orders, others: others}
	rd := &releasingDrivers{routeDriversFake: drivers}
	return co, rd, routerFor(trackingOptions(co, rd, nil), io.Discard)
}

// Al cerrar la última parada el repartidor quedaba ON_ROUTE para siempre: nadie llamaba a Driver Service
// y la planificación respondía "Sin repartidor disponible" aunque el repartidor ya estuviera libre.
func TestDriverResult_LastStopReleasesTheDriver(t *testing.T) {
	_, drivers, h := resultWithRelease()

	rec := do(h, http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, drivers.released, 1)
	assert.Equal(t, trackedDriverID, drivers.released[0].GetDriverId())
	assert.Equal(t, trackedRouteID, drivers.released[0].GetRouteId())
}

func TestDriverResult_OpenStopsKeepTheDriverOnRoute(t *testing.T) {
	pending := &ordersv1.DriverRouteStop{OrderId: "otra-parada", Status: ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED}
	_, drivers, h := resultWithRelease(pending)

	rec := do(h, http.MethodPost, resultPath, resultDeliveredBody, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, drivers.released, "con paradas abiertas el repartidor sigue en ruta")
}

func TestDriverResult_NotDeliveredAndCancelledAlsoCloseTheRoute(t *testing.T) {
	cancelled := &ordersv1.DriverRouteStop{OrderId: "cancelada", Status: ordersv1.OrderStatus_ORDER_STATUS_CANCELLED}
	_, drivers, h := resultWithRelease(cancelled)
	body := `{"outcome":"NOT_DELIVERED","reason":"CLIENT_ABSENT","location":{"latitude":-0.18,"longitude":-78.46}}`

	rec := do(h, http.MethodPost, resultPath, body, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Len(t, drivers.released, 1)
}
