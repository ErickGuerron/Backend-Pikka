package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var errRepoDown = errors.New("repository down")

// failingCatalog falla las operaciones cuyo error se configura y delega el resto en memRepo.
type failingCatalog struct {
	*memRepo
	listErr, getErr, createErr, setActiveErr, adjustErr, movementsErr error
}

func (f *failingCatalog) ListActiveProducts(ctx context.Context) ([]domain.Product, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.memRepo.ListActiveProducts(ctx)
}

func (f *failingCatalog) ListAllProducts(ctx context.Context) ([]domain.Product, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.memRepo.ListAllProducts(ctx)
}

func (f *failingCatalog) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	if f.getErr != nil {
		return domain.Product{}, f.getErr
	}
	return f.memRepo.GetProduct(ctx, id)
}

func (f *failingCatalog) CreateProduct(ctx context.Context, p domain.Product, initial *domain.StockMovement) error {
	if f.createErr != nil {
		return f.createErr
	}
	return f.memRepo.CreateProduct(ctx, p, initial)
}

func (f *failingCatalog) SetActive(ctx context.Context, id uuid.UUID, active bool, now time.Time) error {
	if f.setActiveErr != nil {
		return f.setActiveErr
	}
	return f.memRepo.SetActive(ctx, id, active, now)
}

func (f *failingCatalog) AdjustStock(ctx context.Context, id uuid.UUID, kind domain.MovementType, delta int64, reason string, actor *uuid.UUID, now time.Time) (domain.StockMovement, error) {
	if f.adjustErr != nil {
		return domain.StockMovement{}, f.adjustErr
	}
	return f.memRepo.AdjustStock(ctx, id, kind, delta, reason, actor, now)
}

func (f *failingCatalog) ListMovements(ctx context.Context, id uuid.UUID) ([]domain.StockMovement, error) {
	if f.movementsErr != nil {
		return nil, f.movementsErr
	}
	return f.memRepo.ListMovements(ctx, id)
}

func seedProduct(t *testing.T, repo *memRepo, name string, active bool) domain.Product {
	t.Helper()
	p, _, err := domain.NewProduct(uuid.New(), domain.NewProductInput{Name: name, UnitPrice: price("1")}, nil, t0)
	require.NoError(t, err)
	p.Active = active
	repo.products[p.ID] = &p
	return p
}

func TestListProductsStaffCanIncludeInactive(t *testing.T) {
	repo := newMemRepo()
	seedProduct(t, repo, "Pan", true)
	seedProduct(t, repo, "Queso", false)
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	all, err := svc.ListProducts(context.Background(), operator, true)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	active, err := svc.ListProducts(context.Background(), operator, false)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

func TestListProductsRepositoryErrorsArePropagated(t *testing.T) {
	svc := catalog.NewService(&failingCatalog{memRepo: newMemRepo(), listErr: errRepoDown}, func() time.Time { return t0 })

	_, err := svc.ListProducts(context.Background(), customer, false)
	assert.ErrorIs(t, err, errRepoDown)

	_, err = svc.ListProducts(context.Background(), admin, true)
	assert.ErrorIs(t, err, errRepoDown)

	_, err = svc.ListPublicProducts(context.Background())
	assert.ErrorIs(t, err, errRepoDown)
}

func TestListPublicProductsIsCappedAtFiftyForTheLanding(t *testing.T) {
	repo := newMemRepo()
	for i := 0; i < 60; i++ {
		seedProduct(t, repo, fmt.Sprintf("Producto %02d", i), true)
	}
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	list, err := svc.ListPublicProducts(context.Background())

	require.NoError(t, err)
	assert.Len(t, list, 50)
}

func TestGetProductCustomerSeesInactiveAsMissing(t *testing.T) {
	repo := newMemRepo()
	inactive := seedProduct(t, repo, "Queso", false)
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	_, err := svc.GetProduct(context.Background(), customer, inactive.ID)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	got, err := svc.GetProduct(context.Background(), admin, inactive.ID)
	require.NoError(t, err)
	assert.Equal(t, inactive.ID, got.ID)
}

func TestGetProductRepositoryErrorIsPropagated(t *testing.T) {
	svc := catalog.NewService(&failingCatalog{memRepo: newMemRepo(), getErr: errRepoDown}, func() time.Time { return t0 })

	_, err := svc.GetProduct(context.Background(), admin, uuid.New())

	assert.ErrorIs(t, err, errRepoDown)
}

func TestCreateProductRepositoryErrorIsPropagated(t *testing.T) {
	svc := catalog.NewService(&failingCatalog{memRepo: newMemRepo(), createErr: errRepoDown}, func() time.Time { return t0 })

	_, err := svc.CreateProduct(context.Background(), admin, domain.NewProductInput{Name: "Pan", UnitPrice: price("1")})

	assert.ErrorIs(t, err, errRepoDown)
}

func TestCreateProductRejectsInvalidInputBeforeStoring(t *testing.T) {
	repo := newMemRepo()
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	_, err := svc.CreateProduct(context.Background(), admin, domain.NewProductInput{Name: "", UnitPrice: price("1")})

	assert.ErrorIs(t, err, domain.ErrInvalidProductName)
	assert.Empty(t, repo.products)
}

func TestSetProductActiveRoleAndRepositoryErrors(t *testing.T) {
	repo := newMemRepo()
	p := seedProduct(t, repo, "Pan", true)
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	_, err := svc.SetProductActive(context.Background(), customer, p.ID, false)
	assert.ErrorIs(t, err, catalog.ErrForbidden)

	got, err := svc.SetProductActive(context.Background(), operator, p.ID, false)
	require.NoError(t, err)
	assert.False(t, got.Active)

	_, err = svc.SetProductActive(context.Background(), operator, uuid.New(), true)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	failing := catalog.NewService(&failingCatalog{memRepo: repo, setActiveErr: errRepoDown}, func() time.Time { return t0 })
	_, err = failing.SetProductActive(context.Background(), admin, p.ID, true)
	assert.ErrorIs(t, err, errRepoDown)
}

func TestAdjustStockRoleKindAndRepositoryErrors(t *testing.T) {
	repo := newMemRepo()
	p := seedProduct(t, repo, "Pan", true)
	repo.products[p.ID].StockQuantity = stock(5)
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	_, err := svc.AdjustStock(context.Background(), customer, p.ID, domain.MovementRestock, 1, "")
	assert.ErrorIs(t, err, catalog.ErrForbidden)

	_, err = svc.AdjustStock(context.Background(), admin, p.ID, domain.MovementSale, -1, "")
	assert.ErrorIs(t, err, domain.ErrInvalidMovement, "las ventas las genera el pedido, no el personal")

	_, err = svc.AdjustStock(context.Background(), admin, p.ID, domain.MovementReturn, 1, "")
	assert.ErrorIs(t, err, domain.ErrInvalidMovement)

	failing := catalog.NewService(&failingCatalog{memRepo: repo, adjustErr: errRepoDown}, func() time.Time { return t0 })
	_, err = failing.AdjustStock(context.Background(), admin, p.ID, domain.MovementRestock, 1, "")
	assert.ErrorIs(t, err, errRepoDown)
}

func TestListStockMovementsRoleProductAndRepositoryErrors(t *testing.T) {
	repo := newMemRepo()
	p := seedProduct(t, repo, "Pan", true)
	svc := catalog.NewService(repo, func() time.Time { return t0 })

	_, err := svc.ListStockMovements(context.Background(), customer, p.ID)
	assert.ErrorIs(t, err, catalog.ErrForbidden)

	_, err = svc.ListStockMovements(context.Background(), operator, uuid.New())
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	failing := catalog.NewService(&failingCatalog{memRepo: repo, movementsErr: errRepoDown}, func() time.Time { return t0 })
	_, err = failing.ListStockMovements(context.Background(), operator, p.ID)
	assert.ErrorIs(t, err, errRepoDown)
}

func TestParsePriceAcceptsDecimalTextAndRejectsGarbage(t *testing.T) {
	got, err := catalog.ParsePrice("2.50")
	require.NoError(t, err)
	assert.Equal(t, "2.5", got.String())

	_, err = catalog.ParsePrice("dos")
	assert.ErrorIs(t, err, domain.ErrInvalidPrice)
}
