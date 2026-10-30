package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidSurchargeBase     = errors.New("surcharge base price must be greater than zero")
	ErrInvalidSurchargeIncident = errors.New("surcharge needs a customer, an order and a reason")
)

// SurchargeRate is the penalty over the base price of the customer's next order (ADR 0006, section 1).
var SurchargeRate = decimal.RequireFromString("0.15")

// SurchargeStatus is where an incidence is in its life. PENDING and PENDING_VERIFICATION have no
// amount yet: the 15 % is computed over an order that does not exist when the incidence happens.
type SurchargeStatus string

const (
	SurchargePending             SurchargeStatus = "PENDING"
	SurchargePendingVerification SurchargeStatus = "PENDING_VERIFICATION"
	SurchargeApplied             SurchargeStatus = "APPLIED"
	SurchargePaid                SurchargeStatus = "PAID"
	SurchargeWaived              SurchargeStatus = "WAIVED"
)

// Surcharge is one penalty incidence. OrderID is the order that was not delivered; it is unique, so
// an incidence charges once and never accumulates over the same order (ADR 0006, section 1).
type Surcharge struct {
	ID         string
	CustomerID string
	OrderID    string
	Reason     string
	Status     SurchargeStatus
	Amount     decimal.NullDecimal
	CreatedAt  time.Time
}

// SurchargeFor returns exactly 15 % of the base price, rounded to cents.
func SurchargeFor(basePrice decimal.Decimal) (decimal.Decimal, error) {
	if !basePrice.IsPositive() {
		return decimal.Zero, ErrInvalidSurchargeBase
	}
	return basePrice.Mul(SurchargeRate).Round(2), nil
}

// NewSurchargeIncident records that an order was not delivered because of the customer. It starts
// PENDING and without amount, or PENDING_VERIFICATION when the reason needs an operator (WRONG_ADDRESS).
func NewSurchargeIncident(customerID, orderID, reason string, at time.Time) (Surcharge, error) {
	reason = strings.TrimSpace(reason)
	if customerID == "" || orderID == "" || reason == "" {
		return Surcharge{}, ErrInvalidSurchargeIncident
	}
	status := SurchargePending
	if NotDeliveredReason(reason).NeedsVerification() {
		status = SurchargePendingVerification
	}
	return Surcharge{CustomerID: customerID, OrderID: orderID, Reason: reason, Status: status, CreatedAt: at.UTC()}, nil
}
