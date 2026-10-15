package http_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// fakeDriverClient cuenta las llamadas: las rutas de repartidor aún no deben llegar al servicio.
type fakeDriverClient struct {
	calls int
}

var errNoLlamar = errors.New("el gateway no debe llamar al servicio de repartidores aquí")

func (f *fakeDriverClient) GetDriverByUserID(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	f.calls++
	return nil, errNoLlamar
}

func (f *fakeDriverClient) RegisterDelivery(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.RegisterDeliveryRequest) (*driversv1.RegisterDeliveryResponse, error) {
	f.calls++
	return nil, errNoLlamar
}

type driverRoute struct {
	method, path, body string
}

var driverRoutes = []driverRoute{
	{http.MethodGet, "/api/v1/drivers/me/route", ""},
	{http.MethodPatch, "/api/v1/drivers/me/routes/r-1/stops/s-1/status", `{"status":"PICKED_UP"}`},
	{http.MethodPost, "/api/v1/drivers/me/routes/r-1/stops/s-1/result", `{"outcome":"DELIVERED"}`},
}

func driversRouter(c httpapi.DriverClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Drivers: c})
}

func TestDriverRoutes_WithoutToken_Returns401(t *testing.T) {
	h := driversRouter(&fakeDriverClient{})
	for _, r := range driverRoutes {
		rec := do(h, r.method, r.path, r.body, nil)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, r.method+" "+r.path)
	}
}

func TestDriverRoutes_NonDriverRole_Returns403(t *testing.T) {
	fake := &fakeDriverClient{}
	h := driversRouter(fake)
	for _, role := range []string{"ADMIN", "OPERATOR", "CUSTOMER"} {
		for _, r := range driverRoutes {
			rec := do(h, r.method, r.path, r.body, map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)})
			assert.Equal(t, http.StatusForbidden, rec.Code, role+" "+r.method+" "+r.path)
		}
	}
	assert.Zero(t, fake.calls)
}

// Sin fuente de ruta ni de paradas, las rutas responden 501 explícito y nunca 200.
func TestDriverRoutes_DriverGets501WithoutFakeSuccess(t *testing.T) {
	fake := &fakeDriverClient{}
	h := driversRouter(fake)
	for _, r := range driverRoutes {
		rec := do(h, r.method, r.path, r.body, map[string]string{"Authorization": "Bearer " + token(t, "u-driver", "DRIVER")})
		assert.Equal(t, http.StatusNotImplemented, rec.Code, r.method+" "+r.path)
		assert.Equal(t, "NOT_IMPLEMENTED", decodeError(t, rec).Code, r.method+" "+r.path)
	}
	assert.Zero(t, fake.calls)
}
