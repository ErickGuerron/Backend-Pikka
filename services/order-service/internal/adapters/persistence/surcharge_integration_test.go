//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func newIncident(t *testing.T, customer, order string) domain.Surcharge {
	t.Helper()
	s, err := domain.NewSurchargeIncident(customer, order, "CUSTOMER_REFUSED", time.Now())
	require.NoError(t, err)
	return s
}

func TestRegistrarIncidenciaGuardaPendienteSinMonto(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)

	saved, created, err := repo.RecordSurchargeIncident(context.Background(), newIncident(t, customer, uuid.NewString()))

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, domain.SurchargePending, saved.Status)
	assert.False(t, saved.Amount.Valid)
}

// ADR 0006: una sola vez por incidencia. El segundo intento devuelve la fila existente.
func TestIncidenciaSeRegistraUnaSolaVezPorPedido(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	order := uuid.NewString()
	ctx := context.Background()

	first, created, err := repo.RecordSurchargeIncident(ctx, newIncident(t, customer, order))
	require.NoError(t, err)
	require.True(t, created)

	again, created, err := repo.RecordSurchargeIncident(ctx, newIncident(t, customer, order))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM surcharges WHERE order_id = $1`, order))
}

func TestIncidenciaConcurrenteCreaUnaSolaFila(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	order := uuid.NewString()

	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, created, err := repo.RecordSurchargeIncident(context.Background(), newIncident(t, customer, order))
			assert.NoError(t, err)
			if created {
				mu.Lock()
				createdCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, createdCount)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM surcharges WHERE order_id = $1`, order))
}
