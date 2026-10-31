package domain

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// BalanceDeadlineDays is how long a customer has to pay a surcharge before the account is blocked
// (ADR 0006, section 5: "pasadas 2 semanas").
const BalanceDeadlineDays = 14

// ErrCustomerBlockedBySurcharge: an account blocked for an unpaid balance cannot cancel orders
// (ADR 0006, section 5). The customer can still sign in and see the balance.
var ErrCustomerBlockedBySurcharge = errors.New("account is blocked for an unpaid balance")

var (
	// ErrNothingToPay: the customer owes nothing, so there is no charge to make.
	ErrNothingToPay = errors.New("there is no pending balance to pay")
	// ErrBalanceChanged: the balance moved while the payment was being made; the charge is refunded.
	ErrBalanceChanged = errors.New("the balance changed while paying; try again")
)

// BalanceObligation is one unpaid amount that started a customer's two-week clock. StartedAt is when
// the system confirmed the penalty; the driver never starts the clock.
type BalanceObligation struct {
	CustomerID string
	StartedAt  time.Time
}

// BalanceLineKind says where a line of the customer's balance comes from.
type BalanceLineKind string

const (
	// BalanceLineSurcharge: the 15 % of an order that was not delivered because of the customer.
	BalanceLineSurcharge BalanceLineKind = "SURCHARGE"
	// BalanceLineRecidivism: the extra charge an operator applied to a recidivist.
	BalanceLineRecidivism BalanceLineKind = "RECIDIVISM"
	// BalanceLineReassignment: the fixed fee charged when an order was reassigned at the customer's request.
	BalanceLineReassignment BalanceLineKind = "REASSIGNMENT"
)

// BalanceLine is one thing the customer owes, as the customer sees it: an amount, why, and since when.
// Reason is a code the app can translate. For a recidivism charge it is always RECIDIVISM: the operator's
// justification is an internal audit note and is not shown.
type BalanceLine struct {
	Kind      BalanceLineKind
	ID        string
	OrderID   string
	Reason    string
	Amount    decimal.Decimal
	StartedAt time.Time
}

// DueAt is the last moment to pay before the account is blocked; it matches BalanceOverdue.
func (l BalanceLine) DueAt() time.Time {
	return l.StartedAt.AddDate(0, 0, BalanceDeadlineDays)
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
