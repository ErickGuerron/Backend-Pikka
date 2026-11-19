package domain_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var occurred = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

func pricedSurcharge(t *testing.T, reason string, base string) domain.Surcharge {
	t.Helper()
	s, err := domain.NewSurchargeIncident("c1", "o1", reason, occurred)
	require.NoError(t, err)
	s, err = s.Price(decimal.RequireFromString(base))
	require.NoError(t, err)
	return s
}

// The payment clock starts when the system confirms the penalty (ADR 0006, section 5).
func TestSurchargeClockStartsAtCreationOrAtTheOperatorConfirmation(t *testing.T) {
	s := pricedSurcharge(t, "CLIENT_REFUSED", "20.00")
	assert.True(t, s.ClockStart().Equal(occurred))
	assert.True(t, s.DueAt().Equal(occurred.AddDate(0, 0, 14)))

	confirmed := occurred.AddDate(0, 0, 9)
	s.ResolvedAt = confirmed
	assert.True(t, s.ClockStart().Equal(confirmed), "a confirmed WRONG_ADDRESS starts when the operator confirms")
	assert.True(t, s.DueAt().Equal(confirmed.AddDate(0, 0, 14)))
}

func TestSurchargeAppliedNoticeCarriesReasonAmountAndDueDate(t *testing.T) {
	notice, ok := domain.NewSurchargeAppliedNotification(pricedSurcharge(t, "CLIENT_REFUSED", "20.00"))

	require.True(t, ok)
	assert.Equal(t, domain.NotificationSurchargeApplied, notice.Kind)
	assert.Equal(t, "c1", notice.CustomerID)
	assert.Equal(t, "o1", notice.OrderID)
	assert.Equal(t, "CLIENT_REFUSED,3.00,2026-10-15T08:00:00Z", notice.Detail, "language-free: the app translates it")
}

// ADR 0006: a surcharge waiting for the operator is not notified as definitive, and one that is not owed is not notified.
func TestNoSurchargeNoticeUnlessItIsOwedNow(t *testing.T) {
	priced := pricedSurcharge(t, "CLIENT_REFUSED", "20.00")

	waiting := pricedSurcharge(t, "WRONG_ADDRESS", "20.00")
	assert.Equal(t, domain.SurchargePendingVerification, waiting.Status)
	_, ok := domain.NewSurchargeAppliedNotification(waiting)
	assert.False(t, ok, "PENDING_VERIFICATION")

	waived := priced
	waived.Status = domain.SurchargeWaived
	_, ok = domain.NewSurchargeAppliedNotification(waived)
	assert.False(t, ok, "WAIVED")

	unpriced, err := domain.NewSurchargeIncident("c1", "o1", "CLIENT_REFUSED", occurred)
	require.NoError(t, err)
	_, ok = domain.NewSurchargeAppliedNotification(unpriced)
	assert.False(t, ok, "without amount there is nothing to pay")

	paid := priced
	paid.Status = domain.SurchargePaid
	_, ok = domain.NewSurchargeAppliedNotification(paid)
	assert.False(t, ok, "PAID")
}

func TestConfirmedWrongAddressNoticeUsesTheConfirmationDate(t *testing.T) {
	s := pricedSurcharge(t, "WRONG_ADDRESS", "10.00")
	s.Status = domain.SurchargePending // what CONFIRM does
	s.ResolvedAt = occurred.AddDate(0, 0, 9)

	notice, ok := domain.NewSurchargeAppliedNotification(s)

	require.True(t, ok)
	assert.Equal(t, "WRONG_ADDRESS,1.50,2026-10-24T08:00:00Z", notice.Detail)
}

func TestAccountBlockedNoticeHasNoOrderAndNoDetail(t *testing.T) {
	notice := domain.NewAccountBlockedNotification("c1")

	assert.Equal(t, domain.NotificationAccountBlocked, notice.Kind)
	assert.Equal(t, "c1", notice.CustomerID)
	assert.Empty(t, notice.OrderID, "the block belongs to the account, not to an order")
	assert.Empty(t, notice.Detail)
}

func TestNoticeKindsAreTheContractValues(t *testing.T) {
	assert.Equal(t, "SURCHARGE_APPLIED", string(domain.NotificationSurchargeApplied))
	assert.Equal(t, "ACCOUNT_BLOCKED_BY_BALANCE", string(domain.NotificationAccountBlocked))
}
