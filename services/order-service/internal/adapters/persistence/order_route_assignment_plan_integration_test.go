//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Plan: concurrent assignment of the same orders. Row locks serialize the callers; the losers
// see the orders already ROUTE_ASSIGNED and fail with ErrOrderNotPending, leaving no partial state.
func TestPlan_ConcurrentAssignToRoute_ExactlyOneWins(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(100))
	ids := make([]string, 3)
	for i := range ids {
		o, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
		require.NoError(t, err)
		ids[i] = o.ID
	}
	orders := persistence.NewOrderRepository(pool)
	const workers = 8

	routes := make([]string, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		routes[i] = "plan-race#" + string(rune('a'+i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			seq := 0
			_, errs[i] = orders.AssignToRoute(ctx, ids, func(o domain.Order) (domain.Order, error) {
				seq++
				return o.AssignToRoute(routes[i], seq)
			})
		}()
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range errs {
		if err == nil {
			assert.Equal(t, -1, winner, "only one caller may succeed")
			winner = i
			continue
		}
		assert.ErrorIs(t, err, domain.ErrOrderNotPending)
	}
	require.NotEqual(t, -1, winner, "one caller must win")

	assert.Equal(t, 3, countRows(t, pool, `SELECT count(*) FROM orders WHERE route_id = $1 AND status = 'ROUTE_ASSIGNED'`, routes[winner]), "all orders belong to the winner route")
	assert.Equal(t, 3, countRows(t, pool, `SELECT count(*) FROM orders WHERE route_id IS NOT NULL`), "no order carries a loser route")
	assert.Equal(t, 3, countRows(t, pool, `SELECT count(DISTINCT stop_sequence) FROM orders WHERE route_id = $1`, routes[winner]), "stop sequences are unique")
}
