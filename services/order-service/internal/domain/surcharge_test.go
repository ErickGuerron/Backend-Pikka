package domain_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ADR 0006 sección 1: el recargo es exactamente el 15 % del precio base, redondeado a centavos.
func TestSurchargeForIsExactlyFifteenPercent(t *testing.T) {
	cases := map[string]string{
		"10.00":  "1.50",
		"100":    "15.00",
		"33.33":  "5.00", // 4.9995
		"0.10":   "0.02", // 0.015
		"7.99":   "1.20", // 1.1985
		"1234.5": "185.18",
	}
	for base, want := range cases {
		got, err := domain.SurchargeFor(decimal.RequireFromString(base))
		require.NoError(t, err, base)
		assert.True(t, got.Equal(decimal.RequireFromString(want)), "base %s: got %s, want %s", base, got, want)
	}
}

func TestSurchargeForRejectsNonPositiveBase(t *testing.T) {
	for _, base := range []string{"0", "-5"} {
		_, err := domain.SurchargeFor(decimal.RequireFromString(base))
		assert.ErrorIs(t, err, domain.ErrInvalidSurchargeBase, base)
	}
}

// La incidencia nace sin monto: el 15 % se calcula sobre el siguiente pedido, que aún no existe.
func TestNewSurchargeIncidentStartsPendingWithoutAmount(t *testing.T) {
	s, err := domain.NewSurchargeIncident("c1", "o1", "CUSTOMER_REFUSED", time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.SurchargePending, s.Status)
	assert.False(t, s.Amount.Valid, "sin monto hasta que exista el siguiente pedido")
	assert.Equal(t, "o1", s.OrderID)
}

func TestNewSurchargeIncidentRequiresCustomerOrderAndReason(t *testing.T) {
	_, err := domain.NewSurchargeIncident("", "o1", "CUSTOMER_REFUSED", time.Now())
	assert.ErrorIs(t, err, domain.ErrInvalidSurchargeIncident)
	_, err = domain.NewSurchargeIncident("c1", "", "CUSTOMER_REFUSED", time.Now())
	assert.ErrorIs(t, err, domain.ErrInvalidSurchargeIncident)
	_, err = domain.NewSurchargeIncident("c1", "o1", "  ", time.Now())
	assert.ErrorIs(t, err, domain.ErrInvalidSurchargeIncident)
}
