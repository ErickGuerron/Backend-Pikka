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

	ErrSurchargeNotFound               = errors.New("surcharge not found")
	ErrSurchargeNotPendingVerification = errors.New("surcharge is not waiting for verification")
	ErrUnknownSurchargeDecision        = errors.New("surcharge decision must be CONFIRM or WAIVE")
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

	// Audit of the operator's verification (empty until PENDING_VERIFICATION is resolved).
	ResolvedBy       string
	ResolutionReason string
	ResolvedAt       time.Time
}

// SurchargeDecision is what the operator decides about a PENDING_VERIFICATION surcharge.
type SurchargeDecision string

const (
	// DecisionConfirm: the customer is responsible; the surcharge follows the normal flow.
	DecisionConfirm SurchargeDecision = "CONFIRM"
	// DecisionWaive: the customer is not responsible; no surcharge is charged.
	DecisionWaive SurchargeDecision = "WAIVE"
)

// Resolve closes the verification. Only PENDING_VERIFICATION can be resolved, once, with an audited
// reason (5 to 300 characters) and the operator who decided.
func (s Surcharge) Resolve(decision SurchargeDecision, operatorID, reason string, at time.Time) (Surcharge, error) {
	if s.Status != SurchargePendingVerification {
		return s, ErrSurchargeNotPendingVerification
	}
	var next SurchargeStatus
	switch decision {
	case DecisionConfirm:
		next = SurchargePending
	case DecisionWaive:
		next = SurchargeWaived
	default:
		return s, ErrUnknownSurchargeDecision
	}
	if operatorID == "" {
		return s, ErrInvalidSurchargeIncident
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return s, err
	}
	s.Status, s.ResolvedBy, s.ResolutionReason, s.ResolvedAt = next, operatorID, reason, at.UTC()
	return s, nil
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
