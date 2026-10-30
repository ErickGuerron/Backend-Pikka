package payments_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/payments"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func charge(t *testing.T, gw ports.PaymentGateway, amount string, method domain.PaymentMethod) (domain.PaymentAttempt, error) {
	t.Helper()
	return gw.Charge(context.Background(), ports.ChargeRequest{
		CustomerID: "c1",
		Amount:     decimal.RequireFromString(amount),
		Method:     method,
	})
}

func TestSimulatedChargeApprovesCash(t *testing.T) {
	gw := payments.NewSimulated(func() time.Time { return time.Unix(1700000000, 0) })
	attempt, err := charge(t, gw, "12.50", domain.PaymentCash)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentApproved, attempt.Status)
	assert.Equal(t, domain.PaymentCash, attempt.Method)
	assert.NotEmpty(t, attempt.Reference)
}

func TestSimulatedChargeApprovesCardByDefault(t *testing.T) {
	gw := payments.NewSimulated(time.Now)
	attempt, err := charge(t, gw, "12.50", domain.PaymentCard)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentApproved, attempt.Status)
}

// Regla de prueba documentada: una tarjeta con monto terminado en .99 se rechaza.
func TestSimulatedChargeDeclinesCardAmountEndingIn99(t *testing.T) {
	gw := payments.NewSimulated(time.Now)
	attempt, err := charge(t, gw, "7.99", domain.PaymentCard)
	require.NoError(t, err, "un rechazo simulado es un resultado, no un error")
	assert.Equal(t, domain.PaymentDeclined, attempt.Status)
}

func TestSimulatedChargeNeverDeclinesCash(t *testing.T) {
	gw := payments.NewSimulated(time.Now)
	attempt, err := charge(t, gw, "7.99", domain.PaymentCash)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentApproved, attempt.Status)
}

func TestSimulatedChargeRejectsInvalidAmount(t *testing.T) {
	gw := payments.NewSimulated(time.Now)
	_, err := charge(t, gw, "0", domain.PaymentCard)
	assert.ErrorIs(t, err, domain.ErrInvalidPaymentAmount)
}

func TestSimulatedReferencesAreUnique(t *testing.T) {
	gw := payments.NewSimulated(time.Now)
	a, err := charge(t, gw, "5", domain.PaymentCash)
	require.NoError(t, err)
	b, err := charge(t, gw, "5", domain.PaymentCash)
	require.NoError(t, err)
	assert.NotEqual(t, a.Reference, b.Reference)
}

func TestSimulatedRefundKeepsReferenceAndNeedsOne(t *testing.T) {
	gw := payments.NewSimulated(time.Now)
	attempt, err := charge(t, gw, "5", domain.PaymentCash)
	require.NoError(t, err)

	refund, err := gw.Refund(context.Background(), ports.RefundRequest{Reference: attempt.Reference, CustomerID: "c1", Amount: decimal.NewFromInt(5), Method: domain.PaymentCash})
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentApproved, refund.Status)
	assert.NotEqual(t, attempt.Reference, refund.Reference, "el reembolso tiene su propia referencia")

	_, err = gw.Refund(context.Background(), ports.RefundRequest{Reference: "", CustomerID: "c1", Amount: decimal.NewFromInt(5), Method: domain.PaymentCash})
	assert.ErrorIs(t, err, domain.ErrInvalidPaymentReference)
}
