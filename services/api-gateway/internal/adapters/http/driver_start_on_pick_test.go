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
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// startDriversFake: el repartidor "driver-1" es dueño de la ruta y registra las llamadas a StartRoute.
type startDriversFake struct {
	httpapi.DriverClient
	startCalls int
	gotStart   *driversv1.StartRouteRequest
}

func (f *startDriversFake) GetDriverByUserID(_ context.Context, _ grpcclient.CallMeta, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	return &driversv1.GetDriverByUserIDResponse{Driver: &driversv1.DriverSummary{Id: "driver-1", UserId: req.GetUserId()}}, nil
}

func (f *startDriversFake) GetDriverByRouteID(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.GetDriverByRouteIDRequest) (*driversv1.GetDriverByRouteIDResponse, error) {
	return &driversv1.GetDriverByRouteIDResponse{DriverId: "driver-1"}, nil
}

func (f *startDriversFake) StartRoute(_ context.Context, _ grpcclient.CallMeta, req *driversv1.StartRouteRequest) (*driversv1.StartRouteResponse, error) {
	f.startCalls++
	f.gotStart = req
	return &driversv1.StartRouteResponse{Driver: &driversv1.Driver{Id: "driver-1"}}, nil
}

func TestDriverStop_FirstPickedUpStartsTheRouteBeforeAdvancing(t *testing.T) {
	orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP)}
	drivers := &startDriversFake{}
	h := routerFor(trackingOptions(orders, drivers, nil), io.Discard)

	rec := do(h, http.MethodPatch, stopPath, `{"status":"PICKED_UP"}`, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, drivers.startCalls)
	require.NotNil(t, drivers.gotStart)
	assert.Equal(t, "driver-1", drivers.gotStart.GetDriverId())
	assert.Equal(t, trackedRouteID, drivers.gotStart.GetRouteId())
}

func TestDriverStop_IntransitDoesNotStartTheRoute(t *testing.T) {
	orders := &trackingOrdersFake{advanceRes: advanceOK(ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT)}
	drivers := &startDriversFake{}
	h := routerFor(trackingOptions(orders, drivers, nil), io.Discard)

	rec := do(h, http.MethodPatch, stopPath, `{"status":"IN_TRANSIT"}`, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 0, drivers.startCalls)
}
