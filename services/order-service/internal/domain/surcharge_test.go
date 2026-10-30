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

// ADR 0006 enmienda: WRONG_ADDRESS nace en verificación y sin monto; "ningún cobro" hasta que el operador valide.
func TestNewSurchargeIncidentForWrongAddressStartsPendingVerification(t *testing.T) {
	s, err := domain.NewSurchargeIncident("c1", "o1", "WRONG_ADDRESS", time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.SurchargePendingVerification, s.Status)
	assert.False(t, s.Amount.Valid)
}

func pendingVerification(t *testing.T) domain.Surcharge {
	t.Helper()
	s, err := domain.NewSurchargeIncident("c1", "o1", "WRONG_ADDRESS", time.Now())
	require.NoError(t, err)
	return s
}

// El operador valida con el cliente: CONFIRM lo vuelve un recargo normal; WAIVE lo dispensa. Ambos con motivo auditado.
func TestResolveConfirmMovesToPendingAndKeepsAudit(t *testing.T) {
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	got, err := pendingVerification(t).Resolve(domain.DecisionConfirm, "op-1", "  El cliente estaba fuera de casa  ", at)
	require.NoError(t, err)
	assert.Equal(t, domain.SurchargePending, got.Status)
	assert.Equal(t, "op-1", got.ResolvedBy)
	assert.Equal(t, "El cliente estaba fuera de casa", got.ResolutionReason)
	assert.True(t, got.ResolvedAt.Equal(at))
	assert.False(t, got.Amount.Valid, "el monto se calcula con el siguiente pedido")
}

func TestResolveWaiveClosesTheSurcharge(t *testing.T) {
	got, err := pendingVerification(t).Resolve(domain.DecisionWaive, "op-1", "La direccion era del negocio", time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.SurchargeWaived, got.Status)
}

func TestResolveOnlyFromPendingVerification(t *testing.T) {
	for _, status := range []domain.SurchargeStatus{domain.SurchargePending, domain.SurchargeApplied, domain.SurchargePaid, domain.SurchargeWaived} {
		s := pendingVerification(t)
		s.Status = status
		_, err := s.Resolve(domain.DecisionConfirm, "op-1", "motivo valido", time.Now())
		assert.ErrorIs(t, err, domain.ErrSurchargeNotPendingVerification, string(status))
	}
}

func TestResolveRequiresValidDecisionOperatorAndReason(t *testing.T) {
	_, err := pendingVerification(t).Resolve("MAYBE", "op-1", "motivo valido", time.Now())
	assert.ErrorIs(t, err, domain.ErrUnknownSurchargeDecision)
	_, err = pendingVerification(t).Resolve(domain.DecisionConfirm, "", "motivo valido", time.Now())
	assert.ErrorIs(t, err, domain.ErrInvalidSurchargeIncident)
	_, err = pendingVerification(t).Resolve(domain.DecisionConfirm, "op-1", "abc", time.Now())
	assert.ErrorIs(t, err, domain.ErrReasonRequired)
}
