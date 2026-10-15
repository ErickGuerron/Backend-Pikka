package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var t0 = time.Date(2030, 4, 10, 9, 0, 0, 0, time.UTC)

// memRepo es un CatalogRepository en memoria que aplica las mismas reglas del dominio que la base.
type memRepo struct {
	products  map[uuid.UUID]*domain.Product
	movements map[uuid.UUID][]domain.StockMovement
}

func newMemRepo() *memRepo {
	return &memRepo{products: map[uuid.UUID]*domain.Product{}, movements: map[uuid.UUID][]domain.StockMovement{}}
}

func (r *memRepo) CreateProduct(_ context.Context, p domain.Product, initial *domain.StockMovement) error {
	cp := p
	r.products[p.ID] = &cp
	if initial != nil {
		r.movements[p.ID] = append(r.movements[p.ID], *initial)
	}
	return nil
}

func (r *memRepo) GetProduct(_ context.Context, id uuid.UUID) (domain.Product, error) {
	p, ok := r.products[id]
	if !ok {
		return domain.Product{}, domain.ErrProductNotFound
	}
	return *p, nil
}

func (r *memRepo) ListActiveProducts(_ context.Context) ([]domain.Product, error) {
	var out []domain.Product
	for _, p := range r.products {
		if p.Active {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *memRepo) ListAllProducts(_ context.Context) ([]domain.Product, error) {
	var out []domain.Product
	for _, p := range r.products {
		out = append(out, *p)
	}
	return out, nil
}

func (r *memRepo) ListMovements(_ context.Context, id uuid.UUID) ([]domain.StockMovement, error) {
	return r.movements[id], nil
}

func (r *memRepo) SetActive(_ context.Context, id uuid.UUID, active bool, now time.Time) error {
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

func (r *memRepo) AdjustStock(_ context.Context, id uuid.UUID, kind domain.MovementType, delta int64, reason string, actor *uuid.UUID, now time.Time) (domain.StockMovement, error) {
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

var (
	admin    = catalog.Actor{UserID: uuid.New(), Role: "ADMIN"}
	operator = catalog.Actor{UserID: uuid.New(), Role: "OPERATOR"}
	customer = catalog.Actor{UserID: uuid.New(), Role: "CUSTOMER"}
)

func price(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func stock(n int64) *int64 { return &n }

func newService() (*catalog.Service, *memRepo) {
	repo := newMemRepo()
	return catalog.NewService(repo, func() time.Time { return t0 }), repo
}

func TestCrearProductoSoloPersonal(t *testing.T) {
	svc, _ := newService()

	_, err := svc.CreateProduct(context.Background(), customer, domain.NewProductInput{Name: "Pan", UnitPrice: price("1")})
	assert.ErrorIs(t, err, catalog.ErrForbidden)

	p, err := svc.CreateProduct(context.Background(), operator, domain.NewProductInput{Name: "Pan", UnitPrice: price("1"), InitialStock: stock(4)})
	require.NoError(t, err)
	require.NotNil(t, p.StockQuantity)
	assert.Equal(t, int64(4), *p.StockQuantity)
}

// El caso del negocio: 4 manzanas, un ajuste de −1 por daño y una entrada de +1 = 4.
func TestPersonalAjustaStockConHistorial(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	p, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Manzana", UnitPrice: price("0.50"), InitialStock: stock(4)})
	require.NoError(t, err)

	_, err = svc.AdjustStock(ctx, admin, p.ID, domain.MovementAdjustment, -1, "producto dañado")
	require.NoError(t, err)
	last, err := svc.AdjustStock(ctx, admin, p.ID, domain.MovementRestock, 1, "compra")
	require.NoError(t, err)

	assert.Equal(t, int64(4), last.ResultingStock)
	history, err := svc.ListStockMovements(ctx, operator, p.ID)
	require.NoError(t, err)
	assert.Len(t, history, 3, "entrada inicial, ajuste y entrada")
}

func TestVentaYDevolucionNoSeAjustanABano(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	p, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Pan", UnitPrice: price("1"), InitialStock: stock(4)})
	require.NoError(t, err)

	_, err = svc.AdjustStock(ctx, admin, p.ID, domain.MovementSale, -1, "")
	assert.ErrorIs(t, err, domain.ErrInvalidMovement)
	_, err = svc.AdjustStock(ctx, admin, p.ID, domain.MovementReturn, 1, "")
	assert.ErrorIs(t, err, domain.ErrInvalidMovement)
}

func TestClienteSoloVeProductosActivos(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	active, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Pan", UnitPrice: price("1")})
	require.NoError(t, err)
	hidden, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Jugo", UnitPrice: price("2")})
	require.NoError(t, err)
	_, err = svc.SetProductActive(ctx, admin, hidden.ID, false)
	require.NoError(t, err)

	list, err := svc.ListProducts(ctx, customer, false)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, active.ID, list[0].ID)

	_, err = svc.ListProducts(ctx, customer, true)
	assert.ErrorIs(t, err, catalog.ErrForbidden)

	_, err = svc.GetProduct(ctx, customer, hidden.ID)
	assert.ErrorIs(t, err, domain.ErrProductNotFound, "para el cliente un producto inactivo no existe")
}

func TestListaPublicaSoloActivos(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	active, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Pan", UnitPrice: price("1"), InitialStock: stock(4)})
	require.NoError(t, err)
	hidden, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Jugo", UnitPrice: price("2")})
	require.NoError(t, err)
	_, err = svc.SetProductActive(ctx, admin, hidden.ID, false)
	require.NoError(t, err)

	list, err := svc.ListPublicProducts(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, active.ID, list[0].ID)
	assert.True(t, list[0].Active, "solo se devuelven productos activos")
}

func TestListaPublicaTopaEn50(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	for i := 0; i < 60; i++ {
		_, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Producto", UnitPrice: price("1")})
		require.NoError(t, err)
	}

	list, err := svc.ListPublicProducts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 50)
}

func TestHistorialSoloPersonal(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	p, err := svc.CreateProduct(ctx, admin, domain.NewProductInput{Name: "Pan", UnitPrice: price("1"), InitialStock: stock(1)})
	require.NoError(t, err)

	_, err = svc.ListStockMovements(ctx, customer, p.ID)
	assert.ErrorIs(t, err, catalog.ErrForbidden)
}

func TestPrecioSeConvierteSinFlotantes(t *testing.T) {
	got, err := catalog.ParsePrice("2.50")
	require.NoError(t, err)
	assert.True(t, got.Equal(decimal.RequireFromString("2.5")))

	_, err = catalog.ParsePrice("abc")
	assert.ErrorIs(t, err, domain.ErrInvalidPrice)
}
