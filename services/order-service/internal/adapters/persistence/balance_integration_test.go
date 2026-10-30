//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func obligationsOf(t *testing.T, repo *persistence.BalanceRepository, customer string) []domain.BalanceObligation {
	t.Helper()
	all, err := repo.ListActiveObligations(context.Background())
	require.NoError(t, err)
	var mine []domain.BalanceObligation
	for _, o := range all {
		if o.CustomerID == customer {
			mine = append(mine, o)
		}
	}
	return mine
}

func statusOf(t *testing.T, pool *pgxpool.Pool, customer string) string {
	t.Helper()
	var s string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status FROM customers WHERE id = $1`, customer).Scan(&s))
	return s
}

func setStatus(t *testing.T, pool *pgxpool.Pool, customer, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `UPDATE customers SET status = $2 WHERE id = $1`, customer, status)
	require.NoError(t, err)
}

// Solo cuenta lo que sigue sin cobrarse: PENDING. Dispensadas, en verificacion, aplicadas y pagadas no.
func TestObligacionesSoloIncluyeLosRecargosPendientes(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	for _, status := range []domain.SurchargeStatus{domain.SurchargePending, domain.SurchargeWaived, domain.SurchargePendingVerification, domain.SurchargeApplied, domain.SurchargePaid} {
		insertAt(t, pool, customer, status, at)
	}

	got := obligationsOf(t, repo, customer)

	require.Len(t, got, 1)
	assert.True(t, got[0].StartedAt.Equal(at))
}

// Un WRONG_ADDRESS confirmado empieza su plazo cuando el operador lo confirma, no cuando ocurrio.
func TestObligacionDeDireccionConfirmadaEmpiezaAlConfirmar(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	confirmed := created.AddDate(0, 0, 9)
	_, err := pool.Exec(context.Background(), `
		INSERT INTO surcharges (customer_id, order_id, reason, status, created_at, resolved_by, resolution_reason, resolved_at)
		VALUES ($1, gen_random_uuid(), 'WRONG_ADDRESS', 'PENDING', $2, gen_random_uuid(), 'Confirmado con el cliente', $3)`,
		customer, created, confirmed)
	require.NoError(t, err)

	got := obligationsOf(t, repo, customer)

	require.Len(t, got, 1)
	assert.True(t, got[0].StartedAt.Equal(confirmed))
}

func TestRecargoExtraEsObligacionHastaQueSeMarcaPagado(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	unpaid, paid := newCustomer(t, pool), newCustomer(t, pool)
	for _, c := range []string{unpaid, paid} {
		markRecidivist(t, pool, c)
		_, err := persistence.NewRecidivismRepository(pool).ApplyRecidivismCharge(context.Background(), c, buildCharge(c, "12.50"))
		require.NoError(t, err)
	}
	_, err := pool.Exec(context.Background(), `UPDATE recidivism_charges SET paid_at = now() WHERE customer_id = $1`, paid)
	require.NoError(t, err)

	assert.Len(t, obligationsOf(t, repo, unpaid), 1)
	assert.Empty(t, obligationsOf(t, repo, paid), "ya pagado: no es una obligacion")
}

func TestClientesYaBloqueadosNoTienenObligacionesActivas(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now().AddDate(0, 0, -30))
	setStatus(t, pool, customer, "BLOQUEADO_POR_INCIDENCIAS")

	assert.Empty(t, obligationsOf(t, repo, customer))
}

func TestBloquearPorSaldoSoloMueveClientesActivos(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	active, byIncidents := newCustomer(t, pool), newCustomer(t, pool)
	setStatus(t, pool, byIncidents, "BLOQUEADO_POR_INCIDENCIAS")

	n, err := repo.BlockCustomersForBalance(context.Background(), []string{active, byIncidents, uuid.NewString()})

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, "BLOQUEADO_POR_SALDO", statusOf(t, pool, active))
	assert.Equal(t, "BLOQUEADO_POR_INCIDENCIAS", statusOf(t, pool, byIncidents), "un bloqueo por incidencias no se sobrescribe")
}

func TestBloquearListaVaciaNoHaceNada(t *testing.T) {
	_, pool := setup(t)
	n, err := persistence.NewBalanceRepository(pool).BlockCustomersForBalance(context.Background(), nil)
	require.NoError(t, err)
	assert.Zero(t, n)
}

// De punta a punta con la base real: el de 20 dias se bloquea, el de 3 dias no, y repetirlo no cambia nada.
func TestBarridoBloqueaSoloAlVencido(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	late, fresh := newCustomer(t, pool), newCustomer(t, pool)
	insertAt(t, pool, late, domain.SurchargePending, time.Now().AddDate(0, 0, -20))
	insertAt(t, pool, fresh, domain.SurchargePending, time.Now().AddDate(0, 0, -3))
	sweep := orderapp.NewBlockOverdueBalances(repo)

	n, err := sweep.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, "BLOQUEADO_POR_SALDO", statusOf(t, pool, late))
	assert.Equal(t, "ACTIVO", statusOf(t, pool, fresh))

	again, err := sweep.Run(context.Background())
	require.NoError(t, err)
	assert.Zero(t, again, "el ya bloqueado no se vuelve a tocar")
}
