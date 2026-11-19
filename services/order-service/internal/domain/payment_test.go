package domain_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestParsePaymentMethod(t *testing.T) {
	cash, err := domain.ParsePaymentMethod("CASH")
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentCash, cash)

	card, err := domain.ParsePaymentMethod("CARD")
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentCard, card)

	for _, bad := range []string{"", "cash", "PAYPAL", " CARD"} {
		_, err := domain.ParsePaymentMethod(bad)
		assert.ErrorIs(t, err, domain.ErrInvalidPaymentMethod, "%q", bad)
	}
}

func TestNewPaymentAttemptRequiresPositiveAmount(t *testing.T) {
	for _, amount := range []string{"0", "-1.50"} {
		_, err := domain.NewPaymentAttempt("c1", decimal.RequireFromString(amount), domain.PaymentCash, domain.PaymentApproved, "ref-1", time.Now())
		assert.ErrorIs(t, err, domain.ErrInvalidPaymentAmount, amount)
	}
}

func TestNewPaymentAttemptRoundsToCents(t *testing.T) {
	attempt, err := domain.NewPaymentAttempt("c1", decimal.RequireFromString("1.005"), domain.PaymentCash, domain.PaymentApproved, "ref-1", time.Now())
	require.NoError(t, err)
	assert.True(t, attempt.Amount.Equal(decimal.RequireFromString("1.01")), "got %s", attempt.Amount)
}

func TestNewPaymentAttemptRequiresReference(t *testing.T) {
	_, err := domain.NewPaymentAttempt("c1", decimal.NewFromInt(5), domain.PaymentCard, domain.PaymentDeclined, "  ", time.Now())
	assert.ErrorIs(t, err, domain.ErrInvalidPaymentReference)
}

// ADR 0006 sección 3: el intento de cobro no tiene dónde guardar datos de tarjeta.
func TestPaymentAttemptHasNoCardDataFields(t *testing.T) {
	typ := reflect.TypeOf(domain.PaymentAttempt{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		for _, forbidden := range []string{"pan", "cvv", "cvc", "expiry", "expiration", "cardnumber", "last4"} {
			assert.NotEqual(t, forbidden, name, "campo prohibido en PaymentAttempt")
		}
	}
}

func TestPaymentStatusStrings(t *testing.T) {
	assert.Equal(t, "SIMULATED_APPROVED", string(domain.PaymentApproved))
	assert.Equal(t, "SIMULATED_DECLINED", string(domain.PaymentDeclined))
}
