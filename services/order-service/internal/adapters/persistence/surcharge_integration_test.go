//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

func recordWaiting(t *testing.T, repo *persistence.SurchargeRepository, customer string) domain.Surcharge {
	t.Helper()
	incident, err := domain.NewSurchargeIncident(customer, uuid.NewString(), "WRONG_ADDRESS", time.Now())
	require.NoError(t, err)
	saved, _, err := repo.RecordSurchargeIncident(context.Background(), incident)
	require.NoError(t, err)
	require.Equal(t, domain.SurchargePendingVerification, saved.Status)
	return saved
}

func TestResolverVerificacionGuardaAuditoria(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	waiting := recordWaiting(t, repo, newCustomer(t, pool))
	operator := uuid.NewString()

	got, err := repo.ResolveSurchargeVerification(context.Background(), waiting.ID, func(s domain.Surcharge) (domain.Surcharge, error) {
		return s.Resolve(domain.DecisionWaive, operator, "La direccion era del negocio", time.Now())
	})

	require.NoError(t, err)
	assert.Equal(t, domain.SurchargeWaived, got.Status)
	var status, by, reason string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, resolved_by::text, resolution_reason FROM surcharges WHERE id = $1`, waiting.ID).Scan(&status, &by, &reason))
	assert.Equal(t, "WAIVED", status)
	assert.Equal(t, operator, by)
	assert.Equal(t, "La direccion era del negocio", reason)
}

func TestResolverVerificacionDeIdDesconocidoEsNotFound(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)

	_, err := repo.ResolveSurchargeVerification(context.Background(), uuid.NewString(), func(s domain.Surcharge) (domain.Surcharge, error) { return s, nil })

	assert.ErrorIs(t, err, domain.ErrSurchargeNotFound)
}

// Dos operadores resuelven a la vez: solo uno gana, el otro ve que ya no esta en verificacion.
func TestResolverVerificacionConcurrenteSoloUnoGana(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	waiting := recordWaiting(t, repo, newCustomer(t, pool))

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, losses := 0, 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ResolveSurchargeVerification(context.Background(), waiting.ID, func(s domain.Surcharge) (domain.Surcharge, error) {
				return s.Resolve(domain.DecisionConfirm, uuid.NewString(), "Confirmado con el cliente", time.Now())
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else if assert.ErrorIs(t, err, domain.ErrSurchargeNotPendingVerification) {
				losses++
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, wins)
	assert.Equal(t, 5, losses)
}

func TestListarVerificacionesPendientesSoloDevuelveLasEnEspera(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	waiting := recordWaiting(t, repo, customer)
	resolved := recordWaiting(t, repo, customer)
	_, err := repo.ResolveSurchargeVerification(context.Background(), resolved.ID, func(s domain.Surcharge) (domain.Surcharge, error) {
		return s.Resolve(domain.DecisionConfirm, uuid.NewString(), "Confirmado con el cliente", time.Now())
	})
	require.NoError(t, err)
	_, _, err = repo.RecordSurchargeIncident(context.Background(), newIncident(t, customer, uuid.NewString()))
	require.NoError(t, err)

	list, err := repo.ListSurchargesPendingVerification(context.Background())

	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, waiting.ID, list[0].ID)
}

// insertAt guarda una incidencia con fecha y estado propios, para probar el conteo de reincidencia.
func insertAt(t *testing.T, pool *pgxpool.Pool, customer string, status domain.SurchargeStatus, at time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO surcharges (customer_id, order_id, reason, status, created_at) VALUES ($1, gen_random_uuid(), 'CLIENT_REFUSED', $2, $3)`,
		customer, string(status), at)
	require.NoError(t, err)
}

func TestIncidenciasQueCuentanParaReincidencia(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer, other := newCustomer(t, pool), newCustomer(t, pool)
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	insertAt(t, pool, customer, domain.SurchargePending, base)
	insertAt(t, pool, customer, domain.SurchargeApplied, base.AddDate(0, 0, 2))
	insertAt(t, pool, customer, domain.SurchargePaid, base.AddDate(0, 0, 4))
	insertAt(t, pool, customer, domain.SurchargeWaived, base.AddDate(0, 0, 5))
	insertAt(t, pool, customer, domain.SurchargePendingVerification, base.AddDate(0, 0, 6))
	insertAt(t, pool, other, domain.SurchargePending, base)

	times, err := repo.CountedIncidentTimes(context.Background(), customer)

	require.NoError(t, err)
	require.Len(t, times, 3, "no cuentan las dispensadas, las que esperan verificacion ni las de otro cliente")
	assert.True(t, times[0].Equal(base))
	assert.True(t, domain.ReincidenceReached(times))
}

func TestMarcarReincidenteConservaLaPrimeraFecha(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	first := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

	marked, err := repo.MarkCustomerRecidivist(context.Background(), customer, first)
	require.NoError(t, err)
	assert.True(t, marked)

	again, err := repo.MarkCustomerRecidivist(context.Background(), customer, first.AddDate(0, 0, 3))
	require.NoError(t, err)
	assert.False(t, again, "ya estaba marcado")

	var stored time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT recidivist_at FROM customers WHERE id = $1`, customer).Scan(&stored))
	assert.True(t, stored.Equal(first))
}
