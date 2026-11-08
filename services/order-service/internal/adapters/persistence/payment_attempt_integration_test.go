//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
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

// uniqueRef hace unica la referencia de cobro: la tabla no se vacia entre pruebas, asi que una
// referencia fija choca con la de otra corrida.
func uniqueRef(base string) string { return base + "-" + uuid.NewString() }

func TestGuardarIntentoDeCobroConservaMontoYEstado(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewPaymentAttemptRepository(pool)
	customer := newCustomer(t, pool)

	saved, err := repo.SavePaymentAttempt(context.Background(), newAttempt(t, customer, "3.75", uniqueRef("SIM-1"), domain.PaymentApproved))

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

	_, err := repo.SavePaymentAttempt(context.Background(), newAttempt(t, customer, "7.99", uniqueRef("SIM-2"), domain.PaymentDeclined))
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

	ref := uniqueRef("SIM-3")
	_, err := repo.SavePaymentAttempt(ctx, newAttempt(t, customer, "5", ref, domain.PaymentApproved))
	require.NoError(t, err)
	_, err = repo.SavePaymentAttempt(ctx, newAttempt(t, customer, "5", ref, domain.PaymentApproved))

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

	mineRef := uniqueRef("SIM-4")
	_, err := repo.SavePaymentAttempt(ctx, newAttempt(t, mine, "5", mineRef, domain.PaymentApproved))
	require.NoError(t, err)
	_, err = repo.SavePaymentAttempt(ctx, newAttempt(t, other, "5", uniqueRef("SIM-5"), domain.PaymentApproved))
	require.NoError(t, err)

	list, err := repo.ListPaymentAttempts(ctx, mine)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, mineRef, list[0].Reference)
}
