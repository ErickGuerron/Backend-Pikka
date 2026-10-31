package domain

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidRecidivismCharge = errors.New("recidivism charge needs a customer, an operator and an amount greater than zero")
	ErrCustomerNotRecidivist   = errors.New("customer is not marked as recidivist")
	ErrRecidivismChargeExists  = errors.New("a recidivism charge was already applied for this reincidence")
	// ErrRecidivismChargeTooHigh is wrapped with the configured maximum so the caller can show it.
	ErrRecidivismChargeTooHigh = errors.New("recidivism charge is above the maximum allowed")
	// ErrNotAdmin: only an administrator may remove the reincidence mark.
	ErrNotAdmin               = errors.New("only an administrator can do this")
	ErrInvalidRecidivistClear = errors.New("clearing the reincidence mark needs a customer and an administrator")
)

// RecidivismCharge is the extra surcharge an operator applies to a recidivist customer (ADR 0006,
// section 1: "el operador puede aplicar el cobro con justificacion, sujeto a auditoria"). The amount
// is the operator's decision. It is linked to the reincidence mark that allows it, so it applies once
// per reincidence.
type RecidivismCharge struct {
	ID           string
	CustomerID   string
	RecidivistAt time.Time
	Amount       decimal.Decimal
	Reason       string
	AppliedBy    string
	CreatedAt    time.Time
}

// NewRecidivismCharge validates the operator's decision. The amount is rounded to cents and must stay
// positive; the reason is the audited justification (5 to 300 characters).
func NewRecidivismCharge(customerID string, recidivistAt time.Time, amount decimal.Decimal, operatorID, reason string, at time.Time) (RecidivismCharge, error) {
	amount = amount.Round(2)
	if customerID == "" || operatorID == "" || !amount.IsPositive() {
		return RecidivismCharge{}, ErrInvalidRecidivismCharge
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return RecidivismCharge{}, err
	}
	return RecidivismCharge{
		CustomerID: customerID, RecidivistAt: recidivistAt.UTC(), Amount: amount,
		Reason: reason, AppliedBy: operatorID, CreatedAt: at.UTC(),
	}, nil
}

// Recidivist is what an operator sees about a customer with the reincidence mark: enough to decide an
// extra charge and to reach the customer, and nothing else (no email, no address).
type Recidivist struct {
	CustomerID string
	FullName   string
	// ContactPhone is the phone of the latest order, else the customer's own; empty if there is none.
	ContactPhone     string
	RecidivistAt     time.Time
	PendingSurcharge decimal.Decimal
	// ChargeApplied is true once the extra charge for this mark exists (it can be applied only once).
	ChargeApplied bool
}

// RecidivistClear is the audited removal of the reincidence mark. The charges already collected stay: only
// the mark goes, so a new reincidence can mark the customer again.
type RecidivistClear struct {
	ID           string
	CustomerID   string
	RecidivistAt time.Time // the mark that was removed
	Reason       string
	ClearedBy    string
	CreatedAt    time.Time
}

// NewRecidivistClear validates the administrator's decision; the reason is 5 to 300 characters.
func NewRecidivistClear(customerID string, recidivistAt time.Time, adminID, reason string, at time.Time) (RecidivistClear, error) {
	if customerID == "" || adminID == "" {
		return RecidivistClear{}, ErrInvalidRecidivistClear
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return RecidivistClear{}, err
	}
	return RecidivistClear{
		CustomerID: customerID, RecidivistAt: recidivistAt.UTC(), Reason: reason, ClearedBy: adminID, CreatedAt: at.UTC(),
	}, nil
}
