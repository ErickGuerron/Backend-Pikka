package catalog_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// lockedRepo serializes access to the shared memRepo so concurrent use cases can run under -race.
type lockedRepo struct {
	mu sync.Mutex
	*memRepo
}

func (r *lockedRepo) CreateProduct(ctx context.Context, p domain.Product, m *domain.StockMovement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.memRepo.CreateProduct(ctx, p, m)
}

func (r *lockedRepo) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.memRepo.GetProduct(ctx, id)
}

func (r *lockedRepo) ListAllProducts(ctx context.Context) ([]domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.memRepo.ListAllProducts(ctx)
}

func newPlanService() *catalog.Service {
	return catalog.NewService(&lockedRepo{memRepo: newMemRepo()}, func() time.Time { return t0 })
}

// Plan: "registrar dos productos de forma concurrente".
func TestPlan_CreateProduct_Concurrent_AllSucceedWithDistinctIDs(t *testing.T) {
	const n = 16
	svc := newPlanService()
	start := make(chan struct{})
	ids := make([]uuid.UUID, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			p, err := svc.CreateProduct(context.Background(), admin, domain.NewProductInput{
				Name: "Producto " + uuid.NewString(), UnitPrice: price("1.00"), InitialStock: stock(3),
			})
			ids[i], errs[i] = p.ID, err
		}(i)
	}
	close(start)
	wg.Wait()

	seen := map[uuid.UUID]bool{}
	for i := 0; i < n; i++ {
		require.NoError(t, errs[i])
		assert.False(t, seen[ids[i]], "duplicate id")
		seen[ids[i]] = true
	}
	all, err := svc.ListProducts(context.Background(), admin, true)
	require.NoError(t, err)
	assert.Len(t, all, n)
}

// Characterization: the use case and the in-memory repo enforce no name uniqueness, so two
// products with the same name are both stored with different ids.
func TestPlan_CreateProduct_IdenticalName_CurrentBehavior(t *testing.T) {
	svc := newPlanService()
	ctx := context.Background()
	in := domain.NewProductInput{Name: "Pan", UnitPrice: price("1")}

	a, err := svc.CreateProduct(ctx, admin, in)
	require.NoError(t, err)
	b, err := svc.CreateProduct(ctx, admin, in)

	require.NoError(t, err)
	assert.NotEqual(t, a.ID, b.ID)
	all, err := svc.ListProducts(ctx, admin, true)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestPlan_GetProduct_Nonexistent_NotFound(t *testing.T) {
	svc := newPlanService()
	for _, actor := range []catalog.Actor{admin, customer} {
		_, err := svc.GetProduct(context.Background(), actor, uuid.New())
		assert.ErrorIs(t, err, domain.ErrProductNotFound, actor.Role)
	}
}

func TestPlan_AdjustStock_NonexistentProduct_NotFound(t *testing.T) {
	svc := newPlanService()
	for _, kind := range []domain.MovementType{domain.MovementAdjustment, domain.MovementRestock} {
		_, err := svc.AdjustStock(context.Background(), admin, uuid.New(), kind, 1, "x")
		assert.ErrorIs(t, err, domain.ErrProductNotFound, string(kind))
	}
}
