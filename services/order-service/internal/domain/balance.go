package domain

import (
	"errors"
	"time"
)

// BalanceDeadlineDays is how long a customer has to pay a surcharge before the account is blocked
// (ADR 0006, section 5: "pasadas 2 semanas").
const BalanceDeadlineDays = 14

// ErrCustomerBlockedBySurcharge: an account blocked for an unpaid balance cannot cancel orders
// (ADR 0006, section 5). The customer can still sign in and see the balance.
var ErrCustomerBlockedBySurcharge = errors.New("account is blocked for an unpaid balance")

// BalanceObligation is one unpaid amount that started a customer's two-week clock. StartedAt is when
// the system confirmed the penalty; the driver never starts the clock.
type BalanceObligation struct {
	CustomerID string
	StartedAt  time.Time
}

// BalanceOverdue reports whether the oldest obligation is more than two weeks old at now. The clock of
// the first one is not renewed by newer surcharges. Exactly two weeks is not overdue yet.
func BalanceOverdue(startedAt []time.Time, now time.Time) bool {
	for _, at := range startedAt {
		if now.After(at.AddDate(0, 0, BalanceDeadlineDays)) {
			return true
		}
	}
	return false
}
