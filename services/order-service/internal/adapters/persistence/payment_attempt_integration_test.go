//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func newAttempt(t *testing.T, customer, amount, reference string, status domain.PaymentStatus) domain.PaymentAttempt {
	t.Helper()
	a, err := domain.NewPaymentAttempt(customer, decimal.RequireFromString(amount), domain.PaymentCard, status, reference, time.Now())
	require.NoError(t, err)
	return a
}

func TestGuardarIntentoDeCobroConservaMontoYEstado(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewPaymentAttemptRepository(pool)
	customer := newCustomer(t, pool)

	saved, err := repo.SavePaymentAttempt(context.Background(), newAttempt(t, customer, "3.75", "SIM-1", domain.PaymentApproved))

	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.True(t, saved.Amount.Equal(decimal.RequireFromString("3.75")))
	assert.Equal(t, domain.PaymentApproved, saved.Status)
	assert.Equal(t, domain.PaymentCard, saved.Method)
}

func TestIntentoRechazadoTambienSeRegistra(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewPaymentAttemptRepository(pool)
	customer := newCustomer(t, pool)

	_, err := repo.SavePaymentAttempt(context.Background(), newAttempt(t, customer, "7.99", "SIM-2", domain.PaymentDeclined))
	require.NoError(t, err)

	list, err := repo.ListPaymentAttempts(context.Background(), customer)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, domain.PaymentDeclined, list[0].Status)
}

func TestReferenciaDeCobroEsUnica(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewPaymentAttemptRepository(pool)
	customer := newCustomer(t, pool)
	ctx := context.Background()

	_, err := repo.SavePaymentAttempt(ctx, newAttempt(t, customer, "5", "SIM-3", domain.PaymentApproved))
	require.NoError(t, err)
	_, err = repo.SavePaymentAttempt(ctx, newAttempt(t, customer, "5", "SIM-3", domain.PaymentApproved))

	assert.Error(t, err, "la misma referencia no se registra dos veces")
}

// ADR 0006 sección 3: ninguna columna de la tabla guarda datos de tarjeta.
func TestPaymentAttemptsNoTieneColumnasDeTarjeta(t *testing.T) {
	_, pool := setup(t)
	rows, err := pool.Query(context.Background(), `
		SELECT column_name FROM information_schema.columns
		 WHERE table_name = 'payment_attempts' AND table_schema = current_schema()`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		for _, forbidden := range []string{"card_number", "pan", "cvv", "cvc", "expiry", "expiration", "last4", "last_four"} {
			assert.NotEqual(t, forbidden, name)
		}
	}
	require.NoError(t, rows.Err())
}

func TestListarIntentosSoloDelCliente(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewPaymentAttemptRepository(pool)
	ctx := context.Background()
	mine, other := newCustomer(t, pool), newCustomer(t, pool)

	_, err := repo.SavePaymentAttempt(ctx, newAttempt(t, mine, "5", "SIM-4", domain.PaymentApproved))
	require.NoError(t, err)
	_, err = repo.SavePaymentAttempt(ctx, newAttempt(t, other, "5", "SIM-5", domain.PaymentApproved))
	require.NoError(t, err)

	list, err := repo.ListPaymentAttempts(ctx, mine)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "SIM-4", list[0].Reference)
}
