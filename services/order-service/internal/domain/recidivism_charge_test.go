package domain_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var markedAt = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

func chargeFor(amount, operator, reason string) (domain.RecidivismCharge, error) {
	return domain.NewRecidivismCharge("c1", markedAt, decimal.RequireFromString(amount), operator, reason, time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC))
}

func TestNewRecidivismChargeKeepsTheAudit(t *testing.T) {
	got, err := chargeFor("12.50", "op-1", "  Tres no entregas en dos semanas  ")

	require.NoError(t, err)
	assert.Equal(t, "c1", got.CustomerID)
	assert.True(t, got.RecidivistAt.Equal(markedAt), "queda ligado a la marca que lo permite")
	assert.True(t, got.Amount.Equal(decimal.RequireFromString("12.50")))
	assert.Equal(t, "op-1", got.AppliedBy)
	assert.Equal(t, "Tres no entregas en dos semanas", got.Reason)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestNewRecidivismChargeRoundsToCents(t *testing.T) {
	got, err := chargeFor("3.005", "op-1", "Reincidencia confirmada")
	require.NoError(t, err)
	assert.True(t, got.Amount.Equal(decimal.RequireFromString("3.01")), "got %s", got.Amount)
}

func TestNewRecidivismChargeRejectsInvalidInput(t *testing.T) {
	_, err := chargeFor("0", "op-1", "Reincidencia confirmada")
	assert.ErrorIs(t, err, domain.ErrInvalidRecidivismCharge)
	_, err = chargeFor("-4", "op-1", "Reincidencia confirmada")
	assert.ErrorIs(t, err, domain.ErrInvalidRecidivismCharge)
	_, err = chargeFor("0.004", "op-1", "Reincidencia confirmada")
	assert.ErrorIs(t, err, domain.ErrInvalidRecidivismCharge, "0.004 se redondea a cero")
	_, err = chargeFor("5", "", "Reincidencia confirmada")
	assert.ErrorIs(t, err, domain.ErrInvalidRecidivismCharge)
	_, err = chargeFor("5", "op-1", "abc")
	assert.ErrorIs(t, err, domain.ErrReasonRequired)
}
