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

func buildCharge(customer, amount string) func(time.Time) (domain.RecidivismCharge, error) {
	return func(markedAt time.Time) (domain.RecidivismCharge, error) {
		return domain.NewRecidivismCharge(customer, markedAt, decimal.RequireFromString(amount), "6f9619ff-8b86-d011-b42d-00c04fc964ff", "Tres no entregas en dos semanas", time.Now())
	}
}

func markRecidivist(t *testing.T, pool *pgxpool.Pool, customer string) time.Time {
	t.Helper()
	at := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	_, err := pool.Exec(context.Background(), `UPDATE customers SET recidivist_at = $2 WHERE id = $1`, customer, at)
	require.NoError(t, err)
	return at
}

func balanceOf(t *testing.T, pool *pgxpool.Pool, customer string) decimal.Decimal {
	t.Helper()
	var d decimal.Decimal
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT pending_surcharge FROM customers WHERE id = $1`, customer).Scan(&d))
	return d
}

func TestRecargoExtraSumaAlSaldoYGuardaAuditoria(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markedAt := markRecidivist(t, pool, customer)

	saved, err := repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "12.50"))

	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.True(t, saved.RecidivistAt.Equal(markedAt))
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("12.50")))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM recidivism_charges WHERE customer_id = $1 AND applied_by = '6f9619ff-8b86-d011-b42d-00c04fc964ff'`, customer))
}

func TestRecargoExtraSeAcumulaConElSaldoPendiente(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markRecidivist(t, pool, customer)
	_, err := pool.Exec(context.Background(), `UPDATE customers SET pending_surcharge = 3.00 WHERE id = $1`, customer)
	require.NoError(t, err)

	_, err = repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "10"))

	require.NoError(t, err)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("13.00")))
}

func TestRecargoExtraSinMarcaNoCobraNada(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)

	_, err := repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "12.50"))

	assert.ErrorIs(t, err, domain.ErrCustomerNotRecidivist)
	assert.True(t, balanceOf(t, pool, customer).IsZero())
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM recidivism_charges`))
}

func TestRecargoExtraDeClienteInexistente(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)

	_, err := repo.ApplyRecidivismCharge(context.Background(), "6f9619ff-8b86-d011-b42d-00c04fc964ff", buildCharge("6f9619ff-8b86-d011-b42d-00c04fc964ff", "5"))

	assert.ErrorIs(t, err, domain.ErrCustomerNotFound)
}

func TestSegundoRecargoExtraPorLaMismaMarcaSeRechazaYNoSumaSaldo(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markRecidivist(t, pool, customer)
	_, err := repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "12.50"))
	require.NoError(t, err)

	_, err = repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "20"))

	assert.ErrorIs(t, err, domain.ErrRecidivismChargeExists)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("12.50")), "el saldo no cambia")
}

// Si build rechaza el cobro (por ejemplo un motivo corto) no se guarda nada ni se toca el saldo.
func TestRecargoExtraInvalidoNoDejaRastro(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markRecidivist(t, pool, customer)

	_, err := repo.ApplyRecidivismCharge(context.Background(), customer, func(markedAt time.Time) (domain.RecidivismCharge, error) {
		return domain.NewRecidivismCharge(customer, markedAt, decimal.NewFromInt(5), "op-1", "no", time.Now())
	})

	assert.ErrorIs(t, err, domain.ErrReasonRequired)
	assert.True(t, balanceOf(t, pool, customer).IsZero())
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM recidivism_charges`))
}

func TestRecargoExtraConcurrenteCobraUnaSolaVez(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markRecidivist(t, pool, customer)

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "12.50"))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else {
				assert.ErrorIs(t, err, domain.ErrRecidivismChargeExists)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, wins)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("12.50")), "el saldo suma una sola vez")
}

func TestLeerClienteTraeLaMarcaDeReincidente(t *testing.T) {
	_, pool := setup(t)
	customers := persistence.NewCustomerRepository(pool)
	marked, ordinary := newCustomer(t, pool), newCustomer(t, pool)
	at := markRecidivist(t, pool, marked)

	got, err := customers.GetByID(context.Background(), uuid.MustParse(marked))
	require.NoError(t, err)
	assert.True(t, got.RecidivistAt.Equal(at))

	plain, err := customers.GetByID(context.Background(), uuid.MustParse(ordinary))
	require.NoError(t, err)
	assert.True(t, plain.RecidivistAt.IsZero(), "un cliente normal no tiene marca")
}

func TestListarReincidentesSoloMarcadosLosMasAntiguosPrimeroYSinMasDatosQueLosNecesarios(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	ctx := context.Background()
	older, newer, unmarked := newCustomer(t, pool), newCustomer(t, pool), newCustomer(t, pool)
	_, err := pool.Exec(ctx, `UPDATE customers SET recidivist_at = $2, phone = '0980000000' WHERE id = $1`, older, time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE customers SET recidivist_at = $2, pending_surcharge = 4.50 WHERE id = $1`, newer, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	// newer has an order (phone 0991234567); older has none, so its own phone is the contact.
	product := newProduct(t, store, "Manzana "+uuid.NewString()[:8], stock(50))
	_, err = store.PlaceOrder(ctx, newOrder(t, newer), []domain.OrderLine{lineFor(t, product, 1)}, nil, nil)
	require.NoError(t, err)
	_, err = repo.ApplyRecidivismCharge(ctx, newer, buildCharge(newer, "12.50"))
	require.NoError(t, err)

	got, err := repo.ListRecidivists(ctx)

	require.NoError(t, err)
	var ids []string
	byID := map[string]domain.Recidivist{}
	for _, r := range got {
		ids = append(ids, r.CustomerID)
		byID[r.CustomerID] = r
	}
	assert.NotContains(t, ids, unmarked, "a customer without the mark is not listed")
	assert.Less(t, indexOf(ids, older), indexOf(ids, newer), "oldest mark first")
	assert.Equal(t, "0980000000", byID[older].ContactPhone)
	assert.False(t, byID[older].ChargeApplied)
	assert.Equal(t, "0991234567", byID[newer].ContactPhone, "the latest order's phone wins")
	assert.True(t, byID[newer].ChargeApplied)
	assert.True(t, byID[newer].PendingSurcharge.Equal(decimal.RequireFromString("17.00")), "4.50 owed + 12.50 extra charge")
	assert.True(t, byID[newer].RecidivistAt.Equal(time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)))
}

func indexOf(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}

func buildClear(customer string) func(time.Time) (domain.RecidivistClear, error) {
	return func(markedAt time.Time) (domain.RecidivistClear, error) {
		return domain.NewRecidivistClear(customer, markedAt, "6f9619ff-8b86-d011-b42d-00c04fc964ff", "Pagó todo y se acordó con él", time.Now())
	}
}

func TestQuitarLaMarcaLimpiaLaFechaYDejaAuditoriaSinTocarElSaldoNiLosRecargos(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markedAt := markRecidivist(t, pool, customer)
	_, err := repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "12.50"))
	require.NoError(t, err)

	saved, err := repo.ClearRecidivistMark(context.Background(), customer, buildClear(customer))

	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.True(t, saved.RecidivistAt.Equal(markedAt))
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM customers WHERE id = $1 AND recidivist_at IS NOT NULL`, customer))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM recidivist_clears WHERE customer_id = $1 AND cleared_by = '6f9619ff-8b86-d011-b42d-00c04fc964ff'`, customer))
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("12.50")))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM recidivism_charges WHERE customer_id = $1`, customer))
}

func TestQuitarLaMarcaDeUnClienteSinMarcaOInexistenteNoDejaRastro(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)

	_, err := repo.ClearRecidivistMark(context.Background(), customer, buildClear(customer))
	assert.ErrorIs(t, err, domain.ErrCustomerNotRecidivist)

	ghost := uuid.NewString()
	_, err = repo.ClearRecidivistMark(context.Background(), ghost, buildClear(ghost))
	assert.ErrorIs(t, err, domain.ErrCustomerNotFound)

	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM recidivist_clears`))
}

func TestQuitarLaMarcaDosVecesSoloLaPrimeraTieneEfecto(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markRecidivist(t, pool, customer)

	_, err := repo.ClearRecidivistMark(context.Background(), customer, buildClear(customer))
	require.NoError(t, err)
	_, err = repo.ClearRecidivistMark(context.Background(), customer, buildClear(customer))

	assert.ErrorIs(t, err, domain.ErrCustomerNotRecidivist)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM recidivist_clears WHERE customer_id = $1`, customer))
}

func TestTrasQuitarLaMarcaUnaNuevaReincidenciaPermiteOtroRecargoExtra(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewRecidivismRepository(pool)
	customer := newCustomer(t, pool)
	markRecidivist(t, pool, customer)
	_, err := repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "10"))
	require.NoError(t, err)
	_, err = repo.ClearRecidivistMark(context.Background(), customer, buildClear(customer))
	require.NoError(t, err)
	// A new reincidence marks the customer again, at a later moment.
	_, err = pool.Exec(context.Background(), `UPDATE customers SET recidivist_at = $2 WHERE id = $1`, customer, time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	_, err = repo.ApplyRecidivismCharge(context.Background(), customer, buildCharge(customer, "5"))

	require.NoError(t, err)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("15.00")))
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM recidivism_charges WHERE customer_id = $1`, customer))
}
