//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

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
