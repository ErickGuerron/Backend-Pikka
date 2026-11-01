package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// covCatRepo es un catálogo en memoria con las mismas reglas de dominio que la base.
type covCatRepo struct {
	ports.CatalogRepository
	products  map[uuid.UUID]*domain.Product
	movements map[uuid.UUID][]domain.StockMovement
}

func newCovCatRepo() *covCatRepo {
	return &covCatRepo{products: map[uuid.UUID]*domain.Product{}, movements: map[uuid.UUID][]domain.StockMovement{}}
}

func (r *covCatRepo) CreateProduct(_ context.Context, p domain.Product, initial *domain.StockMovement) error {
	cp := p
	r.products[p.ID] = &cp
	if initial != nil {
		r.movements[p.ID] = append(r.movements[p.ID], *initial)
	}
	return nil
}

func (r *covCatRepo) GetProduct(_ context.Context, id uuid.UUID) (domain.Product, error) {
	p, ok := r.products[id]
	if !ok {
		return domain.Product{}, domain.ErrProductNotFound
	}
	return *p, nil
}

func (r *covCatRepo) ListActiveProducts(_ context.Context) ([]domain.Product, error) {
	var out []domain.Product
	for _, p := range r.products {
		if p.Active {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *covCatRepo) ListAllProducts(_ context.Context) ([]domain.Product, error) {
	var out []domain.Product
	for _, p := range r.products {
		out = append(out, *p)
	}
	return out, nil
}

func (r *covCatRepo) ListMovements(_ context.Context, id uuid.UUID) ([]domain.StockMovement, error) {
	return r.movements[id], nil
}

func (r *covCatRepo) SetActive(_ context.Context, id uuid.UUID, active bool, now time.Time) error {
	p, ok := r.products[id]
	if !ok {
		return domain.ErrProductNotFound
	}
	if active {
		p.Activate(now)
	} else {
		p.Deactivate(now)
	}
	return nil
}

func (r *covCatRepo) AdjustStock(_ context.Context, id uuid.UUID, kind domain.MovementType, delta int64, reason string, actor *uuid.UUID, now time.Time) (domain.StockMovement, error) {
	p, ok := r.products[id]
	if !ok {
		return domain.StockMovement{}, domain.ErrProductNotFound
	}
	m, err := p.ApplyMovement(kind, delta, reason, nil, actor, now)
	if err != nil {
		return domain.StockMovement{}, err
	}
	r.movements[id] = append(r.movements[id], m)
	return m, nil
}

// covCatServer arma el adaptador sobre el caso de uso real y un catálogo en memoria.
func covCatServer(repo *covCatRepo) *CatalogServer {
	return NewCatalogServer(catalog.NewService(repo, time.Now))
}

func covSeed(t *testing.T, repo *covCatRepo, stock *int64, active bool) domain.Product {
	t.Helper()
	p, _, err := domain.NewProduct(uuid.New(), domain.NewProductInput{Name: "Pan", UnitPrice: decimal.RequireFromString("2.50")}, nil, time.Now())
	require.NoError(t, err)
	p.StockQuantity = stock
	p.Active = active
	repo.products[p.ID] = &p
	return p
}

func TestListProductsStaffSeesInactiveWithStockAndTimestamps(t *testing.T) {
	repo := newCovCatRepo()
	stock := int64(7)
	covSeed(t, repo, &stock, true)
	covSeed(t, repo, nil, false)

	res, err := covCatServer(repo).ListProducts(withIdentity("ADMIN"), &catalogv1.ListProductsRequest{IncludeInactive: true})

	require.NoError(t, err)
	require.Len(t, res.GetProducts(), 2)
	var tracked *catalogv1.Product
	for _, p := range res.GetProducts() {
		if p.StockQuantity != nil {
			tracked = p
		}
	}
	require.NotNil(t, tracked, "el producto con stock conserva su cantidad")
	assert.Equal(t, int64(7), tracked.GetStockQuantity())
	assert.Equal(t, "2.50", tracked.GetUnitPrice())
	assert.True(t, tracked.GetActive())
	_, err = time.Parse(time.RFC3339, tracked.GetCreatedAt())
	assert.NoError(t, err, "las fechas viajan en RFC 3339")
	_, err = time.Parse(time.RFC3339, tracked.GetUpdatedAt())
	assert.NoError(t, err)
}

func TestListProductsCustomerSeesOnlyActiveProducts(t *testing.T) {
	repo := newCovCatRepo()
	covSeed(t, repo, nil, true)
	covSeed(t, repo, nil, false)

	res, err := covCatServer(repo).ListProducts(withIdentity("CUSTOMER"), &catalogv1.ListProductsRequest{})

	require.NoError(t, err)
	require.Len(t, res.GetProducts(), 1)
	assert.Nil(t, res.GetProducts()[0].StockQuantity, "sin stock controlado no se envía la cantidad")
}

func TestGetProductHidesInactiveFromCustomers(t *testing.T) {
	repo := newCovCatRepo()
	active := covSeed(t, repo, nil, true)
	inactive := covSeed(t, repo, nil, false)
	srv := covCatServer(repo)

	got, err := srv.GetProduct(withIdentity("CUSTOMER"), &catalogv1.GetProductRequest{Id: active.ID.String()})
	require.NoError(t, err)
	assert.Equal(t, active.ID.String(), got.GetProduct().GetId())

	_, err = srv.GetProduct(withIdentity("CUSTOMER"), &catalogv1.GetProductRequest{Id: inactive.ID.String()})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = srv.GetProduct(withIdentity("OPERATOR"), &catalogv1.GetProductRequest{Id: inactive.ID.String()})
	assert.NoError(t, err, "el personal sí ve productos inactivos")

	_, err = srv.GetProduct(withIdentity("ADMIN"), &catalogv1.GetProductRequest{Id: uuid.NewString()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetProductRequiresIdentity(t *testing.T) {
	_, err := covCatServer(newCovCatRepo()).GetProduct(context.Background(), &catalogv1.GetProductRequest{Id: uuid.NewString()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCreateProductReturnsInitialStock(t *testing.T) {
	repo := newCovCatRepo()
	srv := covCatServer(repo)
	initial := int64(12)

	res, err := srv.CreateProduct(withIdentity("OPERATOR"), &catalogv1.CreateProductRequest{
		Name: "Pan", Description: "Recién horneado", UnitPrice: "1.25", InitialStock: &initial,
	})

	require.NoError(t, err)
	require.NotNil(t, res.GetProduct().StockQuantity)
	assert.Equal(t, int64(12), res.GetProduct().GetStockQuantity())
	assert.Equal(t, "Recién horneado", res.GetProduct().GetDescription())
}

func TestCreateProductErrorCodes(t *testing.T) {
	srv := covCatServer(newCovCatRepo())
	negative := int64(-1)

	_, err := srv.CreateProduct(context.Background(), &catalogv1.CreateProductRequest{Name: "Pan", UnitPrice: "1.00"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = srv.CreateProduct(withIdentity("ADMIN"), &catalogv1.CreateProductRequest{Name: "", UnitPrice: "1.00"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "el nombre es obligatorio")

	_, err = srv.CreateProduct(withIdentity("ADMIN"), &catalogv1.CreateProductRequest{Name: "Pan", UnitPrice: "0"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "el precio debe ser positivo")

	_, err = srv.CreateProduct(withIdentity("ADMIN"), &catalogv1.CreateProductRequest{Name: "Pan", UnitPrice: "1.00", InitialStock: &negative})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestSetProductActiveTogglesAndChecksRole(t *testing.T) {
	repo := newCovCatRepo()
	p := covSeed(t, repo, nil, true)
	srv := covCatServer(repo)

	res, err := srv.SetProductActive(withIdentity("ADMIN"), &catalogv1.SetProductActiveRequest{Id: p.ID.String(), Active: false})
	require.NoError(t, err)
	assert.False(t, res.GetProduct().GetActive())

	_, err = srv.SetProductActive(withIdentity("CUSTOMER"), &catalogv1.SetProductActiveRequest{Id: p.ID.String(), Active: true})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = srv.SetProductActive(withIdentity("ADMIN"), &catalogv1.SetProductActiveRequest{Id: "no-es-uuid", Active: true})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = srv.SetProductActive(withIdentity("ADMIN"), &catalogv1.SetProductActiveRequest{Id: uuid.NewString(), Active: true})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = srv.SetProductActive(context.Background(), &catalogv1.SetProductActiveRequest{Id: p.ID.String(), Active: true})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAdjustStockRecordsRestockAndAdjustment(t *testing.T) {
	repo := newCovCatRepo()
	stock := int64(5)
	p := covSeed(t, repo, &stock, true)
	srv := covCatServer(repo)

	res, err := srv.AdjustStock(withIdentity("OPERATOR"), &catalogv1.AdjustStockRequest{
		ProductId: p.ID.String(), Type: catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK, Delta: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK, res.GetMovement().GetType())
	assert.Equal(t, int64(10), res.GetMovement().GetDelta())
	assert.Equal(t, int64(15), res.GetMovement().GetResultingStock())

	res, err = srv.AdjustStock(withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{
		ProductId: p.ID.String(), Type: catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT, Delta: -3, Reason: "Conteo físico",
	})
	require.NoError(t, err)
	assert.Equal(t, catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT, res.GetMovement().GetType())
	assert.Equal(t, int64(12), res.GetMovement().GetResultingStock())
	assert.Equal(t, "Conteo físico", res.GetMovement().GetReason())
}

func TestAdjustStockErrorCodes(t *testing.T) {
	repo := newCovCatRepo()
	stock := int64(2)
	tracked := covSeed(t, repo, &stock, true)
	untracked := covSeed(t, repo, nil, true)
	srv := covCatServer(repo)
	restock := catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK
	adjustment := catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT

	tests := []struct {
		name string
		ctx  context.Context
		req  *catalogv1.AdjustStockRequest
		want codes.Code
	}{
		{"no identity", context.Background(), &catalogv1.AdjustStockRequest{ProductId: tracked.ID.String(), Type: restock, Delta: 1}, codes.Unauthenticated},
		{"customer cannot adjust", withIdentity("CUSTOMER"), &catalogv1.AdjustStockRequest{ProductId: tracked.ID.String(), Type: restock, Delta: 1}, codes.PermissionDenied},
		{"malformed product id", withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{ProductId: "x", Type: restock, Delta: 1}, codes.InvalidArgument},
		{"unknown movement type", withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{ProductId: tracked.ID.String(), Type: catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_SALE, Delta: -1}, codes.InvalidArgument},
		{"adjustment without reason", withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{ProductId: tracked.ID.String(), Type: adjustment, Delta: 1}, codes.InvalidArgument},
		{"more than available", withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{ProductId: tracked.ID.String(), Type: adjustment, Delta: -50, Reason: "Merma"}, codes.FailedPrecondition},
		{"product does not track stock", withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{ProductId: untracked.ID.String(), Type: restock, Delta: 1}, codes.FailedPrecondition},
		{"unknown product", withIdentity("ADMIN"), &catalogv1.AdjustStockRequest{ProductId: uuid.NewString(), Type: restock, Delta: 1}, codes.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := srv.AdjustStock(tt.ctx, tt.req)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestListStockMovementsReturnsHistoryForStaffOnly(t *testing.T) {
	repo := newCovCatRepo()
	stock := int64(5)
	p := covSeed(t, repo, &stock, true)
	orderID := uuid.New()
	repo.movements[p.ID] = append(repo.movements[p.ID],
		domain.StockMovement{ID: uuid.New(), ProductID: p.ID, Type: domain.MovementSale, Delta: -1, ResultingStock: 4, OrderID: &orderID, CreatedAt: time.Now()},
		domain.StockMovement{ID: uuid.New(), ProductID: p.ID, Type: domain.MovementReturn, Delta: 1, ResultingStock: 5, CreatedAt: time.Now()},
	)
	srv := covCatServer(repo)

	res, err := srv.ListStockMovements(withIdentity("OPERATOR"), &catalogv1.ListStockMovementsRequest{ProductId: p.ID.String()})

	require.NoError(t, err)
	require.Len(t, res.GetMovements(), 2)
	assert.Equal(t, catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_SALE, res.GetMovements()[0].GetType())
	assert.Equal(t, orderID.String(), res.GetMovements()[0].GetOrderId())
	assert.Empty(t, res.GetMovements()[1].GetOrderId(), "un movimiento sin pedido no lleva order_id")
	assert.Equal(t, catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RETURN, res.GetMovements()[1].GetType())

	_, err = srv.ListStockMovements(withIdentity("CUSTOMER"), &catalogv1.ListStockMovementsRequest{ProductId: p.ID.String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = srv.ListStockMovements(withIdentity("ADMIN"), &catalogv1.ListStockMovementsRequest{ProductId: uuid.NewString()})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = srv.ListStockMovements(withIdentity("ADMIN"), &catalogv1.ListStockMovementsRequest{ProductId: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = srv.ListStockMovements(context.Background(), &catalogv1.ListStockMovementsRequest{ProductId: p.ID.String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestToCatalogStatusTranslatesDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"forbidden role", catalog.ErrForbidden, codes.PermissionDenied},
		{"product not found", domain.ErrProductNotFound, codes.NotFound},
		{"insufficient stock", domain.ErrInsufficientStock, codes.FailedPrecondition},
		{"stock not tracked", domain.ErrStockNotTracked, codes.FailedPrecondition},
		{"product inactive", domain.ErrProductInactive, codes.FailedPrecondition},
		{"invalid name", domain.ErrInvalidProductName, codes.InvalidArgument},
		{"invalid description", domain.ErrInvalidProductDescription, codes.InvalidArgument},
		{"invalid price", domain.ErrInvalidPrice, codes.InvalidArgument},
		{"invalid stock", domain.ErrInvalidStock, codes.InvalidArgument},
		{"invalid movement", domain.ErrInvalidMovement, codes.InvalidArgument},
		{"adjustment reason required", domain.ErrAdjustmentReasonRequired, codes.InvalidArgument},
		{"unexpected failure is internal", errors.New("db down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := toCatalogStatus(tt.err)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
	assert.Equal(t, "internal error", status.Convert(toCatalogStatus(errors.New("secreto de base"))).Message(), "el detalle interno no llega al cliente")
}

func TestMovementTypeFromProtoAcceptsOnlyStaffMovements(t *testing.T) {
	got, err := movementTypeFromProto(catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK)
	require.NoError(t, err)
	assert.Equal(t, domain.MovementRestock, got)

	got, err = movementTypeFromProto(catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT)
	require.NoError(t, err)
	assert.Equal(t, domain.MovementAdjustment, got)

	for _, t2 := range []catalogv1.StockMovementType{
		catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_UNSPECIFIED,
		catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_SALE,
		catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RETURN,
	} {
		_, err := movementTypeFromProto(t2)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), t2.String())
	}
}

func TestMovementTypeToProtoMapsEveryDomainType(t *testing.T) {
	tests := map[domain.MovementType]catalogv1.StockMovementType{
		domain.MovementRestock:    catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RESTOCK,
		domain.MovementAdjustment: catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_ADJUSTMENT,
		domain.MovementSale:       catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_SALE,
		domain.MovementReturn:     catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_RETURN,
		domain.MovementType("X"):  catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_UNSPECIFIED,
	}
	for kind, want := range tests {
		assert.Equal(t, want, movementTypeToProto(kind), string(kind))
	}
}

func TestProductsToProtoKeepsEveryProduct(t *testing.T) {
	out := productsToProto([]domain.Product{
		{ID: uuid.New(), Name: "A", UnitPrice: decimal.RequireFromString("1"), CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), Name: "B", UnitPrice: decimal.RequireFromString("2"), CreatedAt: time.Now(), UpdatedAt: time.Now()},
	})
	require.Len(t, out, 2)
	assert.Equal(t, "B", out[1].GetName())
}
