package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// fakeCatalogClient registra la última petición y su identidad, y responde con un producto de muestra.
type fakeCatalogClient struct {
	listReq     *catalogv1.ListProductsRequest
	createReq   *catalogv1.CreateProductRequest
	adjustReq   *catalogv1.AdjustStockRequest
	activeReq   *catalogv1.SetProductActiveRequest
	movementsID string
	meta        grpcclient.CallMeta
}

var sampleProduct = &catalogv1.Product{
	Id: "p-1", Name: "Manzana", UnitPrice: "0.50", Active: true,
	StockQuantity: func() *int64 { n := int64(4); return &n }(),
	CreatedAt:     "2030-04-10T15:00:00Z", UpdatedAt: "2030-04-10T15:00:00Z",
}

func (f *fakeCatalogClient) ListProducts(_ context.Context, m grpcclient.CallMeta, req *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	f.listReq, f.meta = req, m
	return &catalogv1.ListProductsResponse{Products: []*catalogv1.Product{sampleProduct}}, nil
}

func (f *fakeCatalogClient) GetProduct(_ context.Context, m grpcclient.CallMeta, _ *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	f.meta = m
	return &catalogv1.GetProductResponse{Product: sampleProduct}, nil
}

func (f *fakeCatalogClient) CreateProduct(_ context.Context, m grpcclient.CallMeta, req *catalogv1.CreateProductRequest) (*catalogv1.CreateProductResponse, error) {
	f.createReq, f.meta = req, m
	return &catalogv1.CreateProductResponse{Product: sampleProduct}, nil
}

func (f *fakeCatalogClient) SetProductActive(_ context.Context, m grpcclient.CallMeta, req *catalogv1.SetProductActiveRequest) (*catalogv1.SetProductActiveResponse, error) {
	f.activeReq, f.meta = req, m
	return &catalogv1.SetProductActiveResponse{Product: sampleProduct}, nil
}

func (f *fakeCatalogClient) AdjustStock(_ context.Context, m grpcclient.CallMeta, req *catalogv1.AdjustStockRequest) (*catalogv1.AdjustStockResponse, error) {
	f.adjustReq, f.meta = req, m
	return &catalogv1.AdjustStockResponse{Movement: &catalogv1.StockMovement{
		Id: "m-1", ProductId: req.GetProductId(), Type: req.GetType(), Delta: req.GetDelta(), ResultingStock: 5,
		Reason: req.GetReason(), CreatedAt: "2030-04-10T15:00:00Z",
	}}, nil
}

func (f *fakeCatalogClient) ListStockMovements(_ context.Context, m grpcclient.CallMeta, req *catalogv1.ListStockMovementsRequest) (*catalogv1.ListStockMovementsResponse, error) {
	f.movementsID, f.meta = req.GetProductId(), m
	return &catalogv1.ListStockMovementsResponse{}, nil
}

func (f *fakeCatalogClient) ListPublicProducts(_ context.Context, m grpcclient.CallMeta, _ *catalogv1.ListPublicProductsRequest) (*catalogv1.ListPublicProductsResponse, error) {
	f.meta = m
	return &catalogv1.ListPublicProductsResponse{Products: []*catalogv1.PublicProduct{{
		Id: "p-1", Name: "Manzana", Description: "Roja y dulce", UnitPrice: "0.50",
	}}}, nil
}

// emptyPublicCatalog responde un catálogo público vacío.
type emptyPublicCatalog struct{ fakeCatalogClient }

func (f *emptyPublicCatalog) ListPublicProducts(context.Context, grpcclient.CallMeta, *catalogv1.ListPublicProductsRequest) (*catalogv1.ListPublicProductsResponse, error) {
	return &catalogv1.ListPublicProductsResponse{}, nil
}

func catalogRouter(c httpapi.CatalogClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Catalog: c})
}

func TestCatalogoPublico_SinTokenYShapeExacto(t *testing.T) {
	rec := do(catalogRouter(&fakeCatalogClient{}), http.MethodGet, "/api/v1/catalog/products", "", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.NotContains(t, body, `"stockQuantity"`, "la landing no recibe stock")
	assert.NotContains(t, body, `"active"`, "la landing no recibe campos internos")

	var parsed struct {
		Products []map[string]any `json:"products"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Products, 1)
	assert.ElementsMatch(t, []string{"id", "name", "description", "unitPrice"}, mapKeys(parsed.Products[0]))
	assert.Equal(t, "0.50", parsed.Products[0]["unitPrice"])
}

func TestCatalogoPublico_SinResultadosEsArregloVacio(t *testing.T) {
	rec := do(catalogRouter(&emptyPublicCatalog{}), http.MethodGet, "/api/v1/catalog/products", "", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"products":[]}`, rec.Body.String())
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestProductos_ClienteLeeElCatalogo(t *testing.T) {
	fake := &fakeCatalogClient{}
	rec := do(catalogRouter(fake), http.MethodGet, "/api/v1/products", "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Products []struct {
			Name          string `json:"name"`
			UnitPrice     string `json:"unitPrice"`
			StockQuantity *int64 `json:"stockQuantity"`
		} `json:"products"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Products, 1)
	assert.Equal(t, "0.50", body.Products[0].UnitPrice, "el precio viaja como texto, sin perder centavos")
	require.NotNil(t, body.Products[0].StockQuantity)
	assert.Equal(t, "CUSTOMER", fake.meta.Role, "la identidad se propaga al servicio")
	assert.False(t, fake.listReq.GetIncludeInactive())
}

func TestProductos_IncludeInactiveMustBeBoolean(t *testing.T) {
	rec := do(catalogRouter(&fakeCatalogClient{}), http.MethodGet, "/api/v1/products?includeInactive=quizas", "", authHdr(t, "OPERATOR"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProductos_ClienteNoCreaProductos(t *testing.T) {
	fake := &fakeCatalogClient{}
	body := `{"name":"Pan","unitPrice":"1.00"}`
	rec := do(catalogRouter(fake), http.MethodPost, "/api/v1/products", body, authHdr(t, "CUSTOMER"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, fake.createReq, "el middleware corta antes de llegar al servicio")
}

func TestProductos_OperadorCreaConStockInicial(t *testing.T) {
	fake := &fakeCatalogClient{}
	body := `{"name":"Manzana","unitPrice":"0.50","initialStock":4}`
	rec := do(catalogRouter(fake), http.MethodPost, "/api/v1/products", body, authHdr(t, "OPERATOR"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "0.50", fake.createReq.GetUnitPrice())
	require.NotNil(t, fake.createReq.InitialStock)
	assert.Equal(t, int64(4), fake.createReq.GetInitialStock())
}

func TestProductos_ActivoEsObligatorio(t *testing.T) {
	rec := do(catalogRouter(&fakeCatalogClient{}), http.MethodPatch, "/api/v1/products/p-1/active", `{}`, authHdr(t, "ADMIN"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProductos_DesactivaProducto(t *testing.T) {
	fake := &fakeCatalogClient{}
	rec := do(catalogRouter(fake), http.MethodPatch, "/api/v1/products/p-1/active", `{"active":false}`, authHdr(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "p-1", fake.activeReq.GetId())
	assert.False(t, fake.activeReq.GetActive())
}

func TestProductos_AjusteSoloRestockOAjuste(t *testing.T) {
	fake := &fakeCatalogClient{}
	rec := do(catalogRouter(fake), http.MethodPost, "/api/v1/products/p-1/stock-adjustments",
		`{"type":"SALE","delta":-1}`, authHdr(t, "OPERATOR"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, fake.adjustReq)
}

// El caso del negocio: 4 manzanas, un ajuste de −1 por daño y una entrada de +1.
func TestProductos_AjusteManualYEntrada(t *testing.T) {
	fake := &fakeCatalogClient{}
	rec := do(catalogRouter(fake), http.MethodPost, "/api/v1/products/p-1/stock-adjustments",
		`{"type":"ADJUSTMENT","delta":-1,"reason":"producto dañado"}`, authHdr(t, "OPERATOR"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT, fake.adjustReq.GetType())
	assert.Equal(t, "producto dañado", fake.adjustReq.GetReason())

	var body struct {
		Type           string `json:"type"`
		ResultingStock int64  `json:"resultingStock"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ADJUSTMENT", body.Type, "el tipo sale en forma corta")

	rec = do(catalogRouter(fake), http.MethodPost, "/api/v1/products/p-1/stock-adjustments",
		`{"type":"RESTOCK","delta":1}`, authHdr(t, "OPERATOR"))
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK, fake.adjustReq.GetType())
}

func TestProductos_HistorialSoloPersonal(t *testing.T) {
	fake := &fakeCatalogClient{}

	rec := do(catalogRouter(fake), http.MethodGet, "/api/v1/products/p-1/stock-movements", "", authHdr(t, "CUSTOMER"))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = do(catalogRouter(fake), http.MethodGet, "/api/v1/products/p-1/stock-movements", "", authHdr(t, "OPERATOR"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "p-1", fake.movementsID)
}

func TestProductos_SinTokenEs401(t *testing.T) {
	rec := do(catalogRouter(&fakeCatalogClient{}), http.MethodGet, "/api/v1/products", "", nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Los productos del pedido llegan al servicio tal cual, con su cantidad.
func TestCrearPedidoReenviaLosProductos(t *testing.T) {
	fake := &fakeOrderClient{}
	body := `{"address":"Avenida Amazonas 123","deliveryDate":"2030-04-12","windowStart":"09:00","windowEnd":"12:00",` +
		`"items":[{"productId":"p-1","quantity":2}]}`
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", body, authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, fake.createReq.GetItems(), 1)
	assert.Equal(t, "p-1", fake.createReq.GetItems()[0].GetProductId())
	assert.Equal(t, int64(2), fake.createReq.GetItems()[0].GetQuantity())
}
