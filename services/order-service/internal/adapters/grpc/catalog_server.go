package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// CatalogServer implementa catalogv1.CatalogServiceServer delegando en la capa de aplicación.
type CatalogServer struct {
	catalogv1.UnimplementedCatalogServiceServer
	svc *catalog.Service
}

// NewCatalogServer crea el adaptador gRPC del catálogo.
func NewCatalogServer(svc *catalog.Service) *CatalogServer {
	return &CatalogServer{svc: svc}
}

func (s *CatalogServer) ListProducts(ctx context.Context, req *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	actor, err := catalogActor(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.svc.ListProducts(ctx, actor, req.GetIncludeInactive())
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	return &catalogv1.ListProductsResponse{Products: productsToProto(list)}, nil
}

func (s *CatalogServer) GetProduct(ctx context.Context, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	actor, err := catalogActor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseProductID(req.GetId())
	if err != nil {
		return nil, err
	}
	p, err := s.svc.GetProduct(ctx, actor, id)
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	return &catalogv1.GetProductResponse{Product: productToProto(p)}, nil
}

func (s *CatalogServer) CreateProduct(ctx context.Context, req *catalogv1.CreateProductRequest) (*catalogv1.CreateProductResponse, error) {
	actor, err := catalogActor(ctx)
	if err != nil {
		return nil, err
	}
	price, err := catalog.ParsePrice(req.GetUnitPrice())
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	input := domain.NewProductInput{
		Name:        req.GetName(),
		Description: req.GetDescription(),
		UnitPrice:   price,
	}
	if req.InitialStock != nil {
		stock := req.GetInitialStock()
		input.InitialStock = &stock
	}
	p, err := s.svc.CreateProduct(ctx, actor, input)
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	return &catalogv1.CreateProductResponse{Product: productToProto(p)}, nil
}

func (s *CatalogServer) SetProductActive(ctx context.Context, req *catalogv1.SetProductActiveRequest) (*catalogv1.SetProductActiveResponse, error) {
	actor, err := catalogActor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseProductID(req.GetId())
	if err != nil {
		return nil, err
	}
	p, err := s.svc.SetProductActive(ctx, actor, id, req.GetActive())
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	return &catalogv1.SetProductActiveResponse{Product: productToProto(p)}, nil
}

func (s *CatalogServer) AdjustStock(ctx context.Context, req *catalogv1.AdjustStockRequest) (*catalogv1.AdjustStockResponse, error) {
	actor, err := catalogActor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseProductID(req.GetProductId())
	if err != nil {
		return nil, err
	}
	kind, err := movementTypeFromProto(req.GetType())
	if err != nil {
		return nil, err
	}
	m, err := s.svc.AdjustStock(ctx, actor, id, kind, req.GetDelta(), req.GetReason())
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	return &catalogv1.AdjustStockResponse{Movement: movementToProto(m)}, nil
}

func (s *CatalogServer) ListStockMovements(ctx context.Context, req *catalogv1.ListStockMovementsRequest) (*catalogv1.ListStockMovementsResponse, error) {
	actor, err := catalogActor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseProductID(req.GetProductId())
	if err != nil {
		return nil, err
	}
	list, err := s.svc.ListStockMovements(ctx, actor, id)
	if err != nil {
		return nil, toCatalogStatus(err)
	}
	out := make([]*catalogv1.StockMovement, 0, len(list))
	for _, m := range list {
		out = append(out, movementToProto(m))
	}
	return &catalogv1.ListStockMovementsResponse{Movements: out}, nil
}

// catalogActor toma la identidad de la metadata. Sin ella no hay operación.
func catalogActor(ctx context.Context) (catalog.Actor, error) {
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return catalog.Actor{}, err
	}
	return catalog.Actor{UserID: userID, Role: role}, nil
}

func parseProductID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "product id must be a UUID")
	}
	return id, nil
}

func movementTypeFromProto(t catalogv1.StockMovementType) (domain.MovementType, error) {
	switch t {
	case catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK:
		return domain.MovementRestock, nil
	case catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT:
		return domain.MovementAdjustment, nil
	default:
		return "", status.Error(codes.InvalidArgument, "type must be RESTOCK or ADJUSTMENT")
	}
}

// toCatalogStatus traduce errores de aplicación y dominio a los códigos gRPC del contrato.
func toCatalogStatus(err error) error {
	switch {
	case errors.Is(err, catalog.ErrForbidden):
		return status.Error(codes.PermissionDenied, "role cannot manage the catalog")
	case errors.Is(err, domain.ErrProductNotFound):
		return status.Error(codes.NotFound, "product not found")
	case errors.Is(err, domain.ErrInsufficientStock):
		return status.Error(codes.FailedPrecondition, "not enough stock")
	case errors.Is(err, domain.ErrStockNotTracked):
		return status.Error(codes.FailedPrecondition, "product does not track stock")
	case errors.Is(err, domain.ErrProductInactive):
		return status.Error(codes.FailedPrecondition, "product is not available")
	case errors.Is(err, domain.ErrInvalidProductName),
		errors.Is(err, domain.ErrInvalidProductDescription),
		errors.Is(err, domain.ErrInvalidPrice),
		errors.Is(err, domain.ErrInvalidStock),
		errors.Is(err, domain.ErrInvalidMovement),
		errors.Is(err, domain.ErrAdjustmentReasonRequired):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func productsToProto(list []domain.Product) []*catalogv1.Product {
	out := make([]*catalogv1.Product, 0, len(list))
	for _, p := range list {
		out = append(out, productToProto(p))
	}
	return out
}

func productToProto(p domain.Product) *catalogv1.Product {
	out := &catalogv1.Product{
		Id:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		UnitPrice:   p.UnitPrice.StringFixed(2),
		Active:      p.Active,
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if p.StockQuantity != nil {
		stock := *p.StockQuantity
		out.StockQuantity = &stock
	}
	return out
}

func movementToProto(m domain.StockMovement) *catalogv1.StockMovement {
	out := &catalogv1.StockMovement{
		Id:             m.ID.String(),
		ProductId:      m.ProductID.String(),
		Type:           movementTypeToProto(m.Type),
		Delta:          m.Delta,
		ResultingStock: m.ResultingStock,
		Reason:         m.Reason,
		CreatedAt:      m.CreatedAt.UTC().Format(time.RFC3339),
	}
	if m.OrderID != nil {
		out.OrderId = m.OrderID.String()
	}
	return out
}

func movementTypeToProto(t domain.MovementType) catalogv1.StockMovementType {
	switch t {
	case domain.MovementRestock:
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK
	case domain.MovementAdjustment:
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT
	case domain.MovementSale:
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_SALE
	case domain.MovementReturn:
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RETURN
	default:
		return catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_UNSPECIFIED
	}
}
