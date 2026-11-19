package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

func decodeJSON(t *testing.T, data []byte, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(data, dst))
}

func httpapiOptionsWithEverything() httpapi.Options {
	return httpapi.Options{Orders: &fakeOrderClient{}, Drivers: &fakeDriverClient{}}
}

func staffHeader(t *testing.T, role string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)}
}

func TestListOrders_OnlyStaff(t *testing.T) {
	fake := &fakeOrderClient{}
	h := ordersRouter(fake)

	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/api/v1/orders", "", nil).Code)
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		assert.Equal(t, http.StatusForbidden, do(h, http.MethodGet, "/api/v1/orders", "", staffHeader(t, role)).Code, role)
	}
	assert.Zero(t, fake.listCalls)
}

func TestListOrders_StaffGetsEveryOrderAsAPlainArray(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR"} {
		fake := &fakeOrderClient{}

		rec := do(ordersRouter(fake), http.MethodGet, "/api/v1/orders", "", staffHeader(t, role))

		require.Equal(t, http.StatusOK, rec.Code, role)
		var orders []struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			DeliveryDate string `json:"deliveryDate"`
		}
		decodeJSON(t, rec.Body.Bytes(), &orders)
		require.Len(t, orders, 2, role)
		assert.Equal(t, "o-1", orders[0].ID)
		assert.Equal(t, "PENDING_REVIEW", orders[1].Status)
		assert.Equal(t, "2026-10-21", orders[1].DeliveryDate)
		assert.Equal(t, role, fake.meta.Role, "la identidad viaja al Order Service, que decide el permiso")
		assert.Empty(t, fake.listReq.GetDeliveryDate())
		assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, fake.listReq.GetStatus())
	}
}

func TestListOrders_ForwardsTheFilters(t *testing.T) {
	fake := &fakeOrderClient{}

	rec := do(ordersRouter(fake), http.MethodGet, "/api/v1/orders?deliveryDate=2026-10-20&status=PENDING_REVIEW", "", staffHeader(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "2026-10-20", fake.listReq.GetDeliveryDate())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW, fake.listReq.GetStatus())
}

func TestListOrders_UnknownStatusIs400WithoutCallingTheService(t *testing.T) {
	fake := &fakeOrderClient{}

	rec := do(ordersRouter(fake), http.MethodGet, "/api/v1/orders?status=NADA", "", staffHeader(t, "ADMIN"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, fake.listCalls)
}

func TestListOrders_EmptyIsAnEmptyArray(t *testing.T) {
	rec := do(ordersRouter(&emptyOrders{}), http.MethodGet, "/api/v1/orders", "", staffHeader(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[]`, rec.Body.String())
}

type emptyOrders struct{ fakeOrderClient }

func (f *emptyOrders) ListOrders(context.Context, grpcclient.CallMeta, *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	return &ordersv1.ListOrdersResponse{}, nil
}

func TestListOrders_ServiceErrorsAreMapped(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"fecha inválida":         {status.Error(codes.InvalidArgument, "delivery date must be YYYY-MM-DD"), http.StatusBadRequest},
		"sin permiso":            {status.Error(codes.PermissionDenied, "not operator"), http.StatusForbidden},
		"servicio no disponible": {status.Error(codes.Unavailable, "down"), http.StatusServiceUnavailable},
	} {
		rec := do(ordersRouter(&fakeOrderClient{err: tc.err}), http.MethodGet, "/api/v1/orders", "", staffHeader(t, "ADMIN"))
		assert.Equal(t, tc.want, rec.Code, name)
	}
}

// Una ruta que no existe debe responder 404 sea cual sea el rol: antes la atrapaba el grupo de rutas
// del repartidor y un ADMIN recibía 403 "driver role required", que escondía que faltaba el endpoint.
func TestUnknownRoutesAre404ForEveryRole(t *testing.T) {
	h := newRouterWithSignup(&fakeAuth{}, httpapiOptionsWithEverything())
	for _, role := range []string{"ADMIN", "OPERATOR", "CUSTOMER", "DRIVER"} {
		for _, path := range []string{"/api/v1/ruta-que-no-existe", "/api/v1/drivers/no-existe/nada", "/api/v1/orders/no/existe/mas"} {
			rec := do(h, http.MethodGet, path, "", staffHeader(t, role))
			assert.Equal(t, http.StatusNotFound, rec.Code, role+" "+path)
			assert.Equal(t, "NOT_FOUND", decodeError(t, rec).Code, role+" "+path)
		}
	}
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, "/api/v1/ruta-que-no-existe", "", nil).Code, "sin sesión también")
}

// Las rutas que sí existen conservan su protección.
func TestKnownRoutesKeepTheirRoleChecks(t *testing.T) {
	h := newRouterWithSignup(&fakeAuth{}, httpapiOptionsWithEverything())
	assert.Equal(t, http.StatusForbidden, do(h, http.MethodGet, "/api/v1/drivers/me/route", "", staffHeader(t, "ADMIN")).Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/api/v1/drivers/me/route", "", nil).Code)
	assert.Equal(t, http.StatusForbidden, do(h, http.MethodGet, "/api/v1/users", "", staffHeader(t, "OPERATOR")).Code)
}
