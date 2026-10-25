//go:build integration

package persistence_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Plan: 16 concurrent product creations with different names all succeed with distinct ids.
func TestPlan_ConcurrentCreateProduct_AllSucceedWithDistinctIDs(t *testing.T) {
	store, pool := setup(t)
	const workers = 16

	ids := make([]uuid.UUID, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, movement, err := domain.NewProduct(uuid.New(), domain.NewProductInput{
				Name: fmt.Sprintf("Producto concurrente %02d", i), UnitPrice: decimal.RequireFromString("2.50"), InitialStock: stock(5),
			}, nil, now)
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = p.ID
			<-start
			errs[i] = store.CreateProduct(context.Background(), p, movement)
		}()
	}
	close(start)
	wg.Wait()

	distinct := map[uuid.UUID]struct{}{}
	for i := range ids {
		require.NoError(t, errs[i], "worker %d", i)
		distinct[ids[i]] = struct{}{}
	}
	assert.Len(t, distinct, workers)
	assert.Equal(t, workers, countRows(t, pool, `SELECT count(*) FROM products`))
	assert.Equal(t, workers, countRows(t, pool, `SELECT count(*) FROM stock_movements`), "each product keeps its opening movement")
}
