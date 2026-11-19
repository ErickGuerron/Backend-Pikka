package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// upstreamDown simula un servicio interno caído. El gateway lo traduce a 503 UPSTREAM_UNAVAILABLE.
var upstreamDown = status.Error(codes.Unavailable, "upstream down")

// assertUpstreamDown fija el contrato público de un fallo upstream: 503 con código estable.
func assertUpstreamDown(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	assert.Equal(t, "UPSTREAM_UNAVAILABLE", decodeError(t, rec).Code)
}

// Un cuerpo JSON roto se rechaza con 400 antes de llegar al servicio. Los fakes de este archivo
// fallan con upstreamDown, así que un 400 demuestra que el servicio no fue invocado.
func assertMalformedBodyRejected(t *testing.T, h http.Handler, method, path string, headers map[string]string) {
	t.Helper()
	rec := do(h, method, path, "{not json", headers)
	assert.Equal(t, http.StatusBadRequest, rec.Code, method+" "+path+": "+rec.Body.String())
}

// --- Auth -------------------------------------------------------------------

// failingAuth falla en las llamadas que ejercita cada caso; el resto no debe invocarse.
type failingAuth struct {
	httpapi.AuthClient
	err error
}

func (f *failingAuth) CreateUser(context.Context, grpcclient.CallMeta, *authv1.CreateUserRequest) (*authv1.CreateUserResponse, error) {
	return nil, f.err
}

func (f *failingAuth) ListUsers(context.Context, grpcclient.CallMeta, *authv1.ListUsersRequest) (*authv1.ListUsersResponse, error) {
	return nil, f.err
}

// unknownRoleAuth lista un usuario con rol no reconocido (ROLE_UNSPECIFIED): el gateway lo muestra vacío.
type unknownRoleAuth struct{ fakeAuth }

func (f *unknownRoleAuth) ListUsers(context.Context, grpcclient.CallMeta, *authv1.ListUsersRequest) (*authv1.ListUsersResponse, error) {
	return &authv1.ListUsersResponse{Users: []*authv1.User{{Id: "u-x", Email: "x@example.com", Role: authv1.Role_ROLE_UNSPECIFIED}}}, nil
}

func TestAuthHandlers_ServiceAndBodyFailures(t *testing.T) {
	admin := map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")}
	validUser := `{"email":"d@example.com","password":"password123","fullName":"D","role":"DRIVER"}`

	t.Run("createUser rechaza cuerpo malformado", func(t *testing.T) {
		h := newRouterWithSignup(&failingAuth{err: upstreamDown}, httpapi.Options{})
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/users", admin)
	})
	t.Run("createUser propaga el fallo del servicio", func(t *testing.T) {
		h := newRouterWithSignup(&failingAuth{err: upstreamDown}, httpapi.Options{})
		assertUpstreamDown(t, do(h, http.MethodPost, "/api/v1/users", validUser, admin))
	})
	t.Run("signup rechaza cuerpo malformado antes de crear la cuenta", func(t *testing.T) {
		h := newRouterWithSignup(&failingAuth{err: upstreamDown}, httpapi.Options{})
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/auth/signup", nil)
	})
	t.Run("listUsers propaga el fallo del servicio", func(t *testing.T) {
		h := newRouterWithSignup(&failingAuth{err: upstreamDown}, httpapi.Options{})
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/users", "", admin))
	})
	t.Run("updateUser rechaza cuerpo malformado", func(t *testing.T) {
		h := newRouterWithSignup(&failingAuth{err: upstreamDown}, httpapi.Options{})
		assertMalformedBodyRejected(t, h, http.MethodPatch, "/api/v1/users/u-2", admin)
	})
	t.Run("createUser acepta ADMIN y OPERATOR y los devuelve en texto", func(t *testing.T) {
		h := newRouterWithSignup(&fakeAuth{}, httpapi.Options{})
		for _, role := range []string{"ADMIN", "OPERATOR"} {
			rec := do(h, http.MethodPost, "/api/v1/users", `{"email":"a@example.com","password":"password123","fullName":"A","role":"`+role+`"}`, admin)
			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"role":"`+role+`"`)
		}
	})
	t.Run("un rol no reconocido se muestra vacío", func(t *testing.T) {
		h := newRouterWithSignup(&unknownRoleAuth{}, httpapi.Options{})
		rec := do(h, http.MethodGet, "/api/v1/users", "", admin)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"role":""`)
	})
}

// --- Catalog ----------------------------------------------------------------

// failingCatalog falla en cada método de catálogo que ejercita el test.
type failingCatalog struct {
	httpapi.CatalogClient
	err error
}

func (f *failingCatalog) ListPublicProducts(context.Context, grpcclient.CallMeta, *catalogv1.ListPublicProductsRequest) (*catalogv1.ListPublicProductsResponse, error) {
	return nil, f.err
}

func (f *failingCatalog) ListProducts(context.Context, grpcclient.CallMeta, *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	return nil, f.err
}

func (f *failingCatalog) GetProduct(context.Context, grpcclient.CallMeta, *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	return nil, f.err
}

func (f *failingCatalog) CreateProduct(context.Context, grpcclient.CallMeta, *catalogv1.CreateProductRequest) (*catalogv1.CreateProductResponse, error) {
	return nil, f.err
}

func (f *failingCatalog) SetProductActive(context.Context, grpcclient.CallMeta, *catalogv1.SetProductActiveRequest) (*catalogv1.SetProductActiveResponse, error) {
	return nil, f.err
}

func (f *failingCatalog) AdjustStock(context.Context, grpcclient.CallMeta, *catalogv1.AdjustStockRequest) (*catalogv1.AdjustStockResponse, error) {
	return nil, f.err
}

func (f *failingCatalog) ListStockMovements(context.Context, grpcclient.CallMeta, *catalogv1.ListStockMovementsRequest) (*catalogv1.ListStockMovementsResponse, error) {
	return nil, f.err
}

// movementsCatalog devuelve un movimiento de stock para ejercitar el mapeo de la lista.
type movementsCatalog struct{ fakeCatalogClient }

func (f *movementsCatalog) ListStockMovements(context.Context, grpcclient.CallMeta, *catalogv1.ListStockMovementsRequest) (*catalogv1.ListStockMovementsResponse, error) {
	return &catalogv1.ListStockMovementsResponse{Movements: []*catalogv1.StockMovement{{
		Id: "m-9", ProductId: "p-1", Type: catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK,
		Delta: 3, ResultingStock: 7, Reason: "entrada", CreatedAt: "2030-04-10T15:00:00Z",
	}}}, nil
}

func TestCatalogHandlers_ServiceAndBodyFailures(t *testing.T) {
	operator := authHdr(t, "OPERATOR")
	admin := authHdr(t, "ADMIN")
	customer := authHdr(t, "CUSTOMER")
	failing := func() httpapi.CatalogClient { return &failingCatalog{err: upstreamDown} }

	cases := []struct {
		name    string
		method  string
		path    string
		body    string
		headers map[string]string
	}{
		{"catalogo público", http.MethodGet, "/api/v1/catalog/products", "", nil},
		{"listado de productos", http.MethodGet, "/api/v1/products", "", customer},
		{"detalle de producto", http.MethodGet, "/api/v1/products/p-1", "", customer},
		{"alta de producto", http.MethodPost, "/api/v1/products", `{"name":"Pan","unitPrice":"1.00"}`, operator},
		{"activación de producto", http.MethodPatch, "/api/v1/products/p-1/active", `{"active":false}`, admin},
		{"ajuste de stock", http.MethodPost, "/api/v1/products/p-1/stock-adjustments", `{"type":"RESTOCK","delta":1}`, operator},
		{"historial de stock", http.MethodGet, "/api/v1/products/p-1/stock-movements", "", operator},
	}
	for _, tc := range cases {
		t.Run(tc.name+" propaga el fallo del servicio", func(t *testing.T) {
			rec := do(catalogRouter(failing()), tc.method, tc.path, tc.body, tc.headers)
			assertUpstreamDown(t, rec)
		})
	}

	t.Run("alta de producto rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, catalogRouter(failing()), http.MethodPost, "/api/v1/products", operator)
	})
	t.Run("activación rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, catalogRouter(failing()), http.MethodPatch, "/api/v1/products/p-1/active", admin)
	})
	t.Run("ajuste de stock rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, catalogRouter(failing()), http.MethodPost, "/api/v1/products/p-1/stock-adjustments", operator)
	})
	t.Run("includeInactive=true llega al servicio", func(t *testing.T) {
		fake := &fakeCatalogClient{}
		rec := do(catalogRouter(fake), http.MethodGet, "/api/v1/products?includeInactive=true", "", operator)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.True(t, fake.listReq.GetIncludeInactive())
	})
	t.Run("detalle de producto devuelve el producto", func(t *testing.T) {
		rec := do(catalogRouter(&fakeCatalogClient{}), http.MethodGet, "/api/v1/products/p-1", "", customer)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"name":"Manzana"`)
	})
	t.Run("historial de stock devuelve los movimientos", func(t *testing.T) {
		rec := do(catalogRouter(&movementsCatalog{}), http.MethodGet, "/api/v1/products/p-1/stock-movements", "", operator)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `{"movements":[{"id":"m-9","productId":"p-1","type":"RESTOCK","delta":3,"resultingStock":7,"reason":"entrada","createdAt":"2030-04-10T15:00:00Z"}]}`, rec.Body.String())
	})
}

// --- Customers --------------------------------------------------------------

// failingCustomers falla en las consultas y cambios de estado de clientes.
type failingCustomers struct {
	httpapi.CustomerQueryClient
	err error
}

func (f *failingCustomers) ListCustomers(context.Context, grpcclient.CallMeta, *customerv1.ListCustomersRequest) (*customerv1.ListCustomersResponse, error) {
	return nil, f.err
}

func (f *failingCustomers) UpdateStatus(context.Context, grpcclient.CallMeta, *customerv1.UpdateStatusRequest) (*customerv1.UpdateStatusResponse, error) {
	return nil, f.err
}

func TestCustomerHandlers_ServiceAndQueryFailures(t *testing.T) {
	admin := authHdr(t, "ADMIN")
	h := customersRouter(&failingCustomers{err: upstreamDown})

	t.Run("listado propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/customers", "", admin))
	})
	t.Run("offset no numérico es 400", func(t *testing.T) {
		rec := do(h, http.MethodGet, "/api/v1/customers?offset=abc", "", admin)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("cambio de estado propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodPatch, "/api/v1/customers/c-1/status", `{"status":"ACTIVE"}`, admin))
	})
	t.Run("cambio de estado rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPatch, "/api/v1/customers/c-1/status", admin)
	})
}

// --- Orders -----------------------------------------------------------------

// failingOrders falla en las consultas de pedidos que ejercita el test.
type failingOrders struct {
	httpapi.OrderClient
	err error
}

func (f *failingOrders) ListMyOrders(context.Context, grpcclient.CallMeta, *ordersv1.ListMyOrdersRequest) (*ordersv1.ListMyOrdersResponse, error) {
	return nil, f.err
}

func (f *failingOrders) ListOrderExceptions(context.Context, grpcclient.CallMeta, *ordersv1.ListOrderExceptionsRequest) (*ordersv1.ListOrderExceptionsResponse, error) {
	return nil, f.err
}

func (f *failingOrders) ListMyNotifications(context.Context, grpcclient.CallMeta, *ordersv1.ListMyNotificationsRequest) (*ordersv1.ListMyNotificationsResponse, error) {
	return nil, f.err
}

// reviewRecorder registra la decisión que llega a Order Service.
type reviewRecorder struct {
	fakeOrderClient
	got *ordersv1.ReviewOrderRequest
}

func (f *reviewRecorder) ReviewOrder(_ context.Context, _ grpcclient.CallMeta, req *ordersv1.ReviewOrderRequest) (*ordersv1.ReviewOrderResponse, error) {
	f.got = req
	return &ordersv1.ReviewOrderResponse{Order: &ordersv1.Order{Id: req.GetId()}}, nil
}

func TestOrderHandlers_ServiceAndBodyFailures(t *testing.T) {
	customer := authHdr(t, "CUSTOMER")
	operator := authHdr(t, "OPERATOR")
	h := ordersRouter(&failingOrders{err: upstreamDown})

	t.Run("crear pedido rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/orders", customer)
	})
	t.Run("mis pedidos propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/orders/mine", "", customer))
	})
	t.Run("excepciones propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/orders/exceptions", "", operator))
	})
	t.Run("revisión rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/orders/o-1/review", operator)
	})
	t.Run("notificaciones propagan el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/notifications/mine", "", customer))
	})
	t.Run("REJECT llega a Order Service como rechazo", func(t *testing.T) {
		rec := &reviewRecorder{}
		resp := do(ordersRouter(rec), http.MethodPost, "/api/v1/orders/o-1/review",
			`{"action":"REJECT","reason":"direccion inexistente"}`, operator)
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		require.NotNil(t, rec.got)
		assert.Equal(t, ordersv1.ReviewAction_REVIEW_ACTION_REJECT, rec.got.GetAction())
	})
}

// --- Surcharges, disputes, reassignment fee ---------------------------------

// failingSurcharges falla en las operaciones de recargos, disputas, tarifa y saldo que ejercita el test.
type failingSurcharges struct {
	httpapi.SurchargeClient
	err error
}

func (f *failingSurcharges) ListSurchargesPendingVerification(context.Context, grpcclient.CallMeta, *ordersv1.ListSurchargesPendingVerificationRequest) (*ordersv1.ListSurchargesPendingVerificationResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) ResolveSurchargeVerification(context.Context, grpcclient.CallMeta, *ordersv1.ResolveSurchargeVerificationRequest) (*ordersv1.ResolveSurchargeVerificationResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) ClearRecidivist(context.Context, grpcclient.CallMeta, *ordersv1.ClearRecidivistRequest) (*ordersv1.ClearRecidivistResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) SetReassignmentFee(context.Context, grpcclient.CallMeta, *ordersv1.SetReassignmentFeeRequest) (*ordersv1.SetReassignmentFeeResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) GetReassignmentFee(context.Context, grpcclient.CallMeta, *ordersv1.GetReassignmentFeeRequest) (*ordersv1.GetReassignmentFeeResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) ListOpenDisputes(context.Context, grpcclient.CallMeta, *ordersv1.ListOpenDisputesRequest) (*ordersv1.ListOpenDisputesResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) ResolveDispute(context.Context, grpcclient.CallMeta, *ordersv1.ResolveDisputeRequest) (*ordersv1.ResolveDisputeResponse, error) {
	return nil, f.err
}

func (f *failingSurcharges) GetMyBalance(context.Context, grpcclient.CallMeta, *ordersv1.GetMyBalanceRequest) (*ordersv1.GetMyBalanceResponse, error) {
	return nil, f.err
}

func TestSurchargeDisputeAndFeeHandlers_ServiceAndBodyFailures(t *testing.T) {
	admin := authHdr(t, "ADMIN")
	operator := authHdr(t, "OPERATOR")
	customer := authHdr(t, "CUSTOMER")
	h := surchargeRouter(&failingSurcharges{err: upstreamDown})

	t.Run("recargos por verificar propagan el fallo", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/surcharges/pending-verification", "", operator))
	})
	t.Run("verificación de recargo propaga el fallo", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodPost, verifyPath, `{"decision":"CONFIRM","reason":"ok"}`, operator))
	})
	t.Run("quitar reincidencia rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/customers/c-1/recidivist/clear", admin)
	})
	t.Run("fijar tarifa rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPut, "/api/v1/settings/reassignment-fee", admin)
	})
	t.Run("fijar tarifa propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodPut, "/api/v1/settings/reassignment-fee", `{"amount":"2.50","reason":"tarifa"}`, admin))
	})
	t.Run("leer tarifa propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/settings/reassignment-fee", "", operator))
	})
	t.Run("cobro de reasignación rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/orders/o-1/reassignment-charge", operator)
	})
	t.Run("abrir disputa rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/surcharges/s-1/dispute", customer)
	})
	t.Run("listar disputas propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/disputes", "", operator))
	})
	t.Run("resolver disputa rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/disputes/d-1/resolution", operator)
	})
	t.Run("resolver disputa propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodPost, "/api/v1/disputes/d-1/resolution", `{"decision":"ACCEPT","reason":"ok"}`, operator))
	})
	t.Run("saldo propio propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/customers/me/balance", "", customer))
	})
}

// --- Zones ------------------------------------------------------------------

// failingZones falla en las operaciones de zonas que ejercita el test.
type failingZones struct {
	httpapi.ZonesClient
	err error
}

func (f *failingZones) ListCoverageZones(context.Context, grpcclient.CallMeta, *zonesv1.ListCoverageZonesRequest) (*zonesv1.ListCoverageZonesResponse, error) {
	return nil, f.err
}

func (f *failingZones) UpdateZone(context.Context, grpcclient.CallMeta, *zonesv1.UpdateZoneRequest) (*zonesv1.UpdateZoneResponse, error) {
	return nil, f.err
}

func (f *failingZones) GetZone(context.Context, grpcclient.CallMeta, *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	return nil, f.err
}

func (f *failingZones) SetZoneStatus(context.Context, grpcclient.CallMeta, *zonesv1.SetZoneStatusRequest) (*zonesv1.SetZoneStatusResponse, error) {
	return nil, f.err
}

func (f *failingZones) DeleteZone(context.Context, grpcclient.CallMeta, *zonesv1.DeleteZoneRequest) (*zonesv1.DeleteZoneResponse, error) {
	return nil, f.err
}

func TestZoneHandlers_ServiceAndBodyFailures(t *testing.T) {
	admin := authHdr(t, "ADMIN")
	customer := authHdr(t, "CUSTOMER")
	h := newZonesRouter(&failingZones{err: upstreamDown})

	t.Run("cobertura propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/coverage/zones", "", customer))
	})
	t.Run("actualizar zona rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPut, "/api/v1/zones/z-1", admin)
	})
	t.Run("actualizar zona propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodPut, "/api/v1/zones/z-1", `{}`, admin))
	})
	t.Run("crear zona rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPost, "/api/v1/zones", admin)
	})
	t.Run("detalle de zona propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodGet, "/api/v1/zones/z-1", "", admin))
	})
	t.Run("clasificar punto rechaza longitud no numérica", func(t *testing.T) {
		rec := do(h, http.MethodGet, "/api/v1/zones/classify?latitude=-0.18&longitude=abc", "", admin)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("cambiar estado rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodPatch, "/api/v1/zones/z-1/status", admin)
	})
	t.Run("cambiar estado propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodPatch, "/api/v1/zones/z-1/status", `{"status":"INACTIVE","reason":"mantenimiento"}`, admin))
	})
	t.Run("borrar zona rechaza cuerpo malformado", func(t *testing.T) {
		assertMalformedBodyRejected(t, h, http.MethodDelete, "/api/v1/zones/z-1", admin)
	})
	t.Run("borrar zona propaga el fallo del servicio", func(t *testing.T) {
		assertUpstreamDown(t, do(h, http.MethodDelete, "/api/v1/zones/z-1", `{"reason":"duplicada"}`, admin))
	})
}
