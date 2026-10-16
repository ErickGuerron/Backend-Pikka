package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// CatalogClient es lo que los endpoints de productos necesitan del Order Service.
type CatalogClient interface {
	ListProducts(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error)
	ListPublicProducts(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.ListPublicProductsRequest) (*catalogv1.ListPublicProductsResponse, error)
	GetProduct(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error)
	CreateProduct(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.CreateProductRequest) (*catalogv1.CreateProductResponse, error)
	SetProductActive(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.SetProductActiveRequest) (*catalogv1.SetProductActiveResponse, error)
	AdjustStock(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.AdjustStockRequest) (*catalogv1.AdjustStockResponse, error)
	ListStockMovements(ctx context.Context, m grpcclient.CallMeta, req *catalogv1.ListStockMovementsRequest) (*catalogv1.ListStockMovementsResponse, error)
}

// catalogHandlers traduce REST <-> gRPC. Los roles los aplican el middleware (403 temprano)
// y el servicio; aquí solo se propaga la identidad.
type catalogHandlers struct {
	catalog CatalogClient
}

type productDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	UnitPrice     string `json:"unitPrice"`
	Active        bool   `json:"active"`
	StockQuantity *int64 `json:"stockQuantity"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type stockMovementDTO struct {
	ID             string `json:"id"`
	ProductID      string `json:"productId"`
	Type           string `json:"type"`
	Delta          int64  `json:"delta"`
	ResultingStock int64  `json:"resultingStock"`
	Reason         string `json:"reason"`
	OrderID        string `json:"orderId,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

type createProductRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	UnitPrice    string `json:"unitPrice"`
	InitialStock *int64 `json:"initialStock"`
}

type setActiveRequest struct {
	Active *bool `json:"active"`
}

type adjustStockRequest struct {
	Type   string `json:"type"`
	Delta  int64  `json:"delta"`
	Reason string `json:"reason"`
}

type productsResponse struct {
	Products []productDTO `json:"products"`
}

// publicProductDTO es la vista de la landing: nunca stock ni campos internos.
type publicProductDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	UnitPrice   string `json:"unitPrice"`
}

type publicProductsResponse struct {
	Products []publicProductDTO `json:"products"`
}

// listPublic devuelve el catálogo activo a cualquiera, sin token.
func (h *catalogHandlers) listPublic(c echo.Context) error {
	res, err := h.catalog.ListPublicProducts(c.Request().Context(), callMeta(c), &catalogv1.ListPublicProductsRequest{})
	if err != nil {
		return err
	}
	out := make([]publicProductDTO, 0, len(res.GetProducts()))
	for _, p := range res.GetProducts() {
		out = append(out, publicProductDTO{
			ID:          p.GetId(),
			Name:        p.GetName(),
			Description: p.GetDescription(),
			UnitPrice:   p.GetUnitPrice(),
		})
	}
	return c.JSON(http.StatusOK, publicProductsResponse{Products: out})
}

type stockMovementsResponse struct {
	Movements []stockMovementDTO `json:"movements"`
}

// list devuelve el catálogo. includeInactive solo lo acepta el personal; el servicio lo decide.
func (h *catalogHandlers) list(c echo.Context) error {
	includeInactive := false
	if raw := c.QueryParam("includeInactive"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "includeInactive must be true or false")
		}
		includeInactive = v
	}
	res, err := h.catalog.ListProducts(c.Request().Context(), callMeta(c), &catalogv1.ListProductsRequest{IncludeInactive: includeInactive})
	if err != nil {
		return err
	}
	out := make([]productDTO, 0, len(res.GetProducts()))
	for _, p := range res.GetProducts() {
		out = append(out, toProductDTO(p))
	}
	return c.JSON(http.StatusOK, productsResponse{Products: out})
}

func (h *catalogHandlers) get(c echo.Context) error {
	res, err := h.catalog.GetProduct(c.Request().Context(), callMeta(c), &catalogv1.GetProductRequest{Id: c.Param("id")})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toProductDTO(res.GetProduct()))
}

func (h *catalogHandlers) create(c echo.Context) error {
	var req createProductRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.catalog.CreateProduct(c.Request().Context(), callMeta(c), &catalogv1.CreateProductRequest{
		Name:         req.Name,
		Description:  req.Description,
		UnitPrice:    req.UnitPrice,
		InitialStock: req.InitialStock,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toProductDTO(res.GetProduct()))
}

func (h *catalogHandlers) setActive(c echo.Context) error {
	var req setActiveRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	if req.Active == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "active is required")
	}
	res, err := h.catalog.SetProductActive(c.Request().Context(), callMeta(c), &catalogv1.SetProductActiveRequest{
		Id:     c.Param("id"),
		Active: *req.Active,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toProductDTO(res.GetProduct()))
}

// adjustStock registra una entrada (RESTOCK) o un ajuste manual con motivo (ADJUSTMENT).
func (h *catalogHandlers) adjustStock(c echo.Context) error {
	var req adjustStockRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	kind, err := movementTypeFromREST(req.Type)
	if err != nil {
		return err
	}
	res, err := h.catalog.AdjustStock(c.Request().Context(), callMeta(c), &catalogv1.AdjustStockRequest{
		ProductId: c.Param("id"),
		Type:      kind,
		Delta:     req.Delta,
		Reason:    req.Reason,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toStockMovementDTO(res.GetMovement()))
}

func (h *catalogHandlers) movements(c echo.Context) error {
	res, err := h.catalog.ListStockMovements(c.Request().Context(), callMeta(c), &catalogv1.ListStockMovementsRequest{ProductId: c.Param("id")})
	if err != nil {
		return err
	}
	out := make([]stockMovementDTO, 0, len(res.GetMovements()))
	for _, m := range res.GetMovements() {
		out = append(out, toStockMovementDTO(m))
	}
	return c.JSON(http.StatusOK, stockMovementsResponse{Movements: out})
}

func movementTypeFromREST(raw string) (catalogv1.StockMovementType, error) {
	switch raw {
	case "RESTOCK":
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK, nil
	case "ADJUSTMENT":
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT, nil
	default:
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_UNSPECIFIED,
			echo.NewHTTPError(http.StatusBadRequest, "type must be RESTOCK or ADJUSTMENT")
	}
}

func toProductDTO(p *catalogv1.Product) productDTO {
	return productDTO{
		ID:            p.GetId(),
		Name:          p.GetName(),
		Description:   p.GetDescription(),
		UnitPrice:     p.GetUnitPrice(),
		Active:        p.GetActive(),
		StockQuantity: p.StockQuantity,
		CreatedAt:     p.GetCreatedAt(),
		UpdatedAt:     p.GetUpdatedAt(),
	}
}

func toStockMovementDTO(m *catalogv1.StockMovement) stockMovementDTO {
	return stockMovementDTO{
		ID:             m.GetId(),
		ProductID:      m.GetProductId(),
		Type:           strings.TrimPrefix(m.GetType().String(), "STOCK_MOVEMENT_TYPE_"),
		Delta:          m.GetDelta(),
		ResultingStock: m.GetResultingStock(),
		Reason:         m.GetReason(),
		OrderID:        m.GetOrderId(),
		CreatedAt:      m.GetCreatedAt(),
	}
}
