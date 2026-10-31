//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

const disputeOperator = "6f9619ff-8b86-d011-b42d-00c04fc964ff"

// owedSurcharge leaves a 3.00 surcharge owed since createdAt and adds it to the customer's balance, like the real flow.
func owedSurcharge(t *testing.T, pool *pgxpool.Pool, customer string, createdAt time.Time) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO surcharges (customer_id, order_id, reason, status, amount, created_at)
		VALUES ($1, gen_random_uuid(), 'CLIENT_REFUSED', 'PENDING', 3.00, $2) RETURNING id::text`, customer, createdAt).Scan(&id))
	_, err := pool.Exec(context.Background(), `UPDATE customers SET pending_surcharge = pending_surcharge + 3.00 WHERE id = $1`, customer)
	require.NoError(t, err)
	return id
}

func openFor(surchargeID string) func(domain.Surcharge) (domain.Dispute, error) {
	return func(s domain.Surcharge) (domain.Dispute, error) {
		return domain.NewDispute(s, "Yo estaba en casa y nadie tocó", time.Now())
	}
}

func decide(decision domain.DisputeStatus) func(domain.Dispute) (domain.Dispute, error) {
	return func(d domain.Dispute) (domain.Dispute, error) {
		return d.Resolve(decision, disputeOperator, "Revisamos la ruta del repartidor", time.Now())
	}
}

func surchargeStatusOf(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var s string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status FROM surcharges WHERE id = $1`, id).Scan(&s))
	return s
}

func TestAbrirUnaDisputaGuardaLaDisputaYNoTocaElRecargoNiElSaldo(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))

	saved, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))

	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.Equal(t, domain.DisputeOpen, saved.Status)
	assert.Equal(t, sid, saved.SurchargeID)
	assert.Equal(t, customer, saved.CustomerID)
	assert.Equal(t, "PENDING", surchargeStatusOf(t, pool, sid))
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("3.00")))
}

func TestUnaSegundaDisputaDelMismoRecargoSeRechaza(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))
	_, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))
	require.NoError(t, err)

	_, err = repo.OpenDispute(context.Background(), sid, customer, openFor(sid))

	assert.ErrorIs(t, err, domain.ErrDisputeExists)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM surcharge_disputes WHERE surcharge_id = $1`, sid))
}

func TestNadieDisputaElRecargoDeOtroNiUnoInexistenteYLaRespuestaEsLaMisma(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	owner, intruder := newCustomer(t, pool), newCustomer(t, pool)
	sid := owedSurcharge(t, pool, owner, time.Now().Add(-10*time.Minute))

	_, err := repo.OpenDispute(context.Background(), sid, intruder, openFor(sid))
	assert.ErrorIs(t, err, domain.ErrSurchargeNotFound)

	ghost := uuid.NewString()
	_, err = repo.OpenDispute(context.Background(), ghost, intruder, openFor(ghost))
	assert.ErrorIs(t, err, domain.ErrSurchargeNotFound)

	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM surcharge_disputes`))
}

func TestFueraDeLaVentanaNoSeGuardaNada(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-3*time.Hour))

	_, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))

	assert.ErrorIs(t, err, domain.ErrDisputeWindowClosed)
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM surcharge_disputes`))
}

func TestElBarridoNoCuentaElRecargoConDisputaAbiertaYVuelveAContarSiSeResuelveEnContra(t *testing.T) {
	_, pool := setup(t)
	disputes := persistence.NewDisputeRepository(pool)
	balances := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	// Overdue for two weeks, but disputed: it must not make the account blockable.
	sid := owedSurcharge(t, pool, customer, time.Now().AddDate(0, 0, -20))
	counted := func() int {
		all, err := balances.ListActiveObligations(context.Background())
		require.NoError(t, err)
		n := 0
		for _, o := range all {
			if o.CustomerID == customer {
				n++
			}
		}
		return n
	}
	require.Equal(t, 1, counted())

	// A late dispute is rejected by the window, so the open one is written straight for this scenario.
	_, err := pool.Exec(context.Background(), `INSERT INTO surcharge_disputes (surcharge_id, customer_id, reason) VALUES ($1, $2, 'Yo estaba en casa')`, sid, customer)
	require.NoError(t, err)
	assert.Equal(t, 0, counted(), "an open dispute pauses the clock")

	var disputeID string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id::text FROM surcharge_disputes WHERE surcharge_id = $1`, sid).Scan(&disputeID))
	_, err = disputes.ResolveDispute(context.Background(), disputeID, decide(domain.DisputeAgainst))
	require.NoError(t, err)
	assert.Equal(t, 1, counted(), "against the customer, the surcharge counts again and keeps its original clock")
}

func TestResolverAFavorDispensaElRecargoYLoQuitaDelSaldoConAuditoria(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	keep := owedSurcharge(t, pool, customer, time.Now().Add(-5*time.Hour))
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))
	opened, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))
	require.NoError(t, err)

	resolved, err := repo.ResolveDispute(context.Background(), opened.ID, decide(domain.DisputeInFavor))

	require.NoError(t, err)
	assert.Equal(t, domain.DisputeInFavor, resolved.Status)
	assert.Equal(t, disputeOperator, resolved.ResolvedBy)
	assert.NotEmpty(t, resolved.ResolutionReason)
	assert.Equal(t, "WAIVED", surchargeStatusOf(t, pool, sid))
	assert.Equal(t, "PENDING", surchargeStatusOf(t, pool, keep), "only the disputed surcharge is waived")
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("3.00")), "6.00 - 3.00")
}

func TestResolverEnContraDejaElRecargoYElSaldoComoEstaban(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))
	opened, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))
	require.NoError(t, err)

	resolved, err := repo.ResolveDispute(context.Background(), opened.ID, decide(domain.DisputeAgainst))

	require.NoError(t, err)
	assert.Equal(t, domain.DisputeAgainst, resolved.Status)
	assert.Equal(t, "PENDING", surchargeStatusOf(t, pool, sid))
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("3.00")))
}

func TestResolverAFavorUnRecargoYaPagadoFallaYLaDisputaSigueAbierta(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))
	opened, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))
	require.NoError(t, err)
	_, err = persistence.NewBalanceRepository(pool).SettleBalance(context.Background(), customer, decimal.RequireFromString("3.00"), time.Now())
	require.NoError(t, err)

	_, err = repo.ResolveDispute(context.Background(), opened.ID, decide(domain.DisputeInFavor))

	assert.ErrorIs(t, err, domain.ErrSurchargeNotDisputable)
	assert.Equal(t, "PAID", surchargeStatusOf(t, pool, sid))
	assert.True(t, balanceOf(t, pool, customer).IsZero(), "the balance must not go negative")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM surcharge_disputes WHERE id = $1 AND status = 'OPEN'`, opened.ID))
}

func TestResolverDosVecesOUnaInexistente(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))
	opened, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))
	require.NoError(t, err)
	_, err = repo.ResolveDispute(context.Background(), opened.ID, decide(domain.DisputeInFavor))
	require.NoError(t, err)

	_, err = repo.ResolveDispute(context.Background(), opened.ID, decide(domain.DisputeInFavor))
	assert.ErrorIs(t, err, domain.ErrDisputeAlreadyResolved)
	assert.True(t, balanceOf(t, pool, customer).IsZero(), "waived once, not twice")

	_, err = repo.ResolveDispute(context.Background(), uuid.NewString(), decide(domain.DisputeInFavor))
	assert.ErrorIs(t, err, domain.ErrDisputeNotFound)
}

func TestDosPersonalResolviendoLaMismaDisputaSoloUnoTieneEfecto(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	customer := newCustomer(t, pool)
	sid := owedSurcharge(t, pool, customer, time.Now().Add(-10*time.Minute))
	opened, err := repo.OpenDispute(context.Background(), sid, customer, openFor(sid))
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = repo.ResolveDispute(context.Background(), opened.ID, decide(domain.DisputeInFavor))
		}(i)
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		if err == nil {
			ok++
		} else {
			assert.ErrorIs(t, err, domain.ErrDisputeAlreadyResolved)
		}
	}
	assert.Equal(t, 1, ok)
	assert.True(t, balanceOf(t, pool, customer).IsZero(), "the surcharge came off the balance exactly once")
}

func TestLaColaDelPersonalSoloTieneLasAbiertasLasMasAntiguasPrimero(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewDisputeRepository(pool)
	a, b := newCustomer(t, pool), newCustomer(t, pool)
	first := owedSurcharge(t, pool, a, time.Now().Add(-50*time.Minute))
	second := owedSurcharge(t, pool, b, time.Now().Add(-40*time.Minute))
	closed := owedSurcharge(t, pool, a, time.Now().Add(-30*time.Minute))
	for _, c := range []struct{ s, cust string }{{first, a}, {second, b}, {closed, a}} {
		_, err := repo.OpenDispute(context.Background(), c.s, c.cust, openFor(c.s))
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}
	var closedID string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id::text FROM surcharge_disputes WHERE surcharge_id = $1`, closed).Scan(&closedID))
	_, err := repo.ResolveDispute(context.Background(), closedID, decide(domain.DisputeAgainst))
	require.NoError(t, err)

	list, err := repo.ListOpenDisputes(context.Background())

	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, first, list[0].SurchargeID)
	assert.Equal(t, second, list[1].SurchargeID)
}
