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
		INSERT INTO surcharges (customer_id, order_id, reason, status, amount, created_at, resolved_by, resolution_reason, resolved_at)
		VALUES ($1, gen_random_uuid(), 'WRONG_ADDRESS', 'PENDING', 1.50, $2, gen_random_uuid(), 'Confirmado con el cliente', $3)`,
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

func setBalance(t *testing.T, pool *pgxpool.Pool, customer, amount string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `UPDATE customers SET pending_surcharge = $2 WHERE id = $1`, customer, amount)
	require.NoError(t, err)
}

func surchargeStatuses(t *testing.T, pool *pgxpool.Pool, customer string) map[string]int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT status, count(*) FROM surcharges WHERE customer_id = $1 GROUP BY status`, customer)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		require.NoError(t, rows.Scan(&status, &n))
		out[status] = n
	}
	require.NoError(t, rows.Err())
	return out
}

// ADR 0006 seccion 5: al pagar el saldo completo la cuenta vuelve a ACTIVO en ese mismo momento.
func TestPagarSaldoMarcaPagadoYDesbloquea(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now().AddDate(0, 0, -20))
	insertAt(t, pool, customer, domain.SurchargeApplied, time.Now().AddDate(0, 0, -2))
	markRecidivist(t, pool, customer)
	_, err := persistence.NewRecidivismRepository(pool).ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "4.00"))
	require.NoError(t, err)
	setBalance(t, pool, customer, "10.00")
	setStatus(t, pool, customer, "BLOQUEADO_POR_SALDO")

	unblocked, err := repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("10.00"), time.Now())

	require.NoError(t, err)
	assert.True(t, unblocked)
	assert.Equal(t, "ACTIVO", statusOf(t, pool, customer))
	assert.True(t, balanceOf(t, pool, customer).IsZero())
	assert.Equal(t, map[string]int{"PAID": 2}, surchargeStatuses(t, pool, customer))
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM recidivism_charges WHERE customer_id = $1 AND paid_at IS NULL`, customer))
}

func TestPagarCuentaActivaNoReportaDesbloqueo(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now())
	setBalance(t, pool, customer, "3.00")

	unblocked, err := repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("3.00"), time.Now())

	require.NoError(t, err)
	assert.False(t, unblocked)
	assert.Equal(t, "ACTIVO", statusOf(t, pool, customer))
}

func TestPagarNoLevantaUnBloqueoPorIncidencias(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now())
	setBalance(t, pool, customer, "3.00")
	setStatus(t, pool, customer, "BLOQUEADO_POR_INCIDENCIAS")

	unblocked, err := repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("3.00"), time.Now())

	require.NoError(t, err)
	assert.False(t, unblocked)
	assert.Equal(t, "BLOQUEADO_POR_INCIDENCIAS", statusOf(t, pool, customer))
	assert.True(t, balanceOf(t, pool, customer).IsZero(), "igual se cobra el saldo")
}

func TestPagarConSaldoDistintoFallaSinCambiarNada(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now().AddDate(0, 0, -20))
	setBalance(t, pool, customer, "3.00")
	setStatus(t, pool, customer, "BLOQUEADO_POR_SALDO")

	_, err := repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("2.00"), time.Now())

	assert.ErrorIs(t, err, domain.ErrBalanceChanged)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("3.00")))
	assert.Equal(t, "BLOQUEADO_POR_SALDO", statusOf(t, pool, customer))
	assert.Equal(t, map[string]int{"PENDING": 1}, surchargeStatuses(t, pool, customer))
}

func TestPagarClienteInexistente(t *testing.T) {
	_, pool := setup(t)
	_, err := persistence.NewBalanceRepository(pool).SettleBalance(context.Background(), uuid.NewString(), decimal.NewFromInt(1), time.Now())
	assert.ErrorIs(t, err, domain.ErrCustomerNotFound)
}

// No se tocan las dispensadas, las que esperan al operador ni las que nunca tuvieron monto.
func TestPagarSoloMarcaLasIncidenciasQueSeCobraron(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now())
	insertAt(t, pool, customer, domain.SurchargeWaived, time.Now())
	insertAt(t, pool, customer, domain.SurchargePendingVerification, time.Now())
	_, err := pool.Exec(context.Background(),
		`INSERT INTO surcharges (customer_id, order_id, reason, status) VALUES ($1, gen_random_uuid(), 'CLIENT_REFUSED', 'PENDING')`, customer)
	require.NoError(t, err)
	setBalance(t, pool, customer, "3.00")

	_, err = repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("3.00"), time.Now())

	require.NoError(t, err)
	assert.Equal(t, map[string]int{"PAID": 1, "WAIVED": 1, "PENDING_VERIFICATION": 1, "PENDING": 1}, surchargeStatuses(t, pool, customer),
		"la sin monto sigue PENDING: nunca se sumo al saldo")
}

// Dos pagos a la vez por el mismo saldo: solo uno se aplica, el otro ve que ya no debe nada.
func TestPagarConcurrenteSoloUnoSeAplica(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now())
	setBalance(t, pool, customer, "3.00")

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("3.00"), time.Now())
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else {
				assert.ErrorIs(t, err, domain.ErrBalanceChanged)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, wins)
	assert.True(t, balanceOf(t, pool, customer).IsZero())
}

// Una incidencia sin monto no se puede pagar, asi que tampoco puede bloquear la cuenta.
func TestIncidenciaSinMontoNoEsObligacion(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO surcharges (customer_id, order_id, reason, status, created_at) VALUES ($1, gen_random_uuid(), 'CLIENT_REFUSED', 'PENDING', $2)`,
		customer, time.Now().AddDate(0, 0, -30))
	require.NoError(t, err)

	assert.Empty(t, obligationsOf(t, repo, customer))
}

// De punta a punta: vencido -> bloqueado -> paga -> activo, y el barrido siguiente no lo vuelve a bloquear.
func TestVencidoBloqueadoPagaYElBarridoNoLoRebloquea(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now().AddDate(0, 0, -20))
	setBalance(t, pool, customer, "3.00")
	sweep := orderapp.NewBlockOverdueBalances(repo)

	blocked, err := sweep.Run(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, blocked)
	require.Equal(t, "BLOQUEADO_POR_SALDO", statusOf(t, pool, customer))

	unblocked, err := repo.SettleBalance(context.Background(), customer, decimal.RequireFromString("3.00"), time.Now())
	require.NoError(t, err)
	require.True(t, unblocked)

	again, err := sweep.Run(context.Background())
	require.NoError(t, err)
	assert.Zero(t, again)
	assert.Equal(t, "ACTIVO", statusOf(t, pool, customer))
}
