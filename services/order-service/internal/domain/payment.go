package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidPaymentMethod    = errors.New("payment method must be CASH or CARD")
	ErrInvalidPaymentAmount    = errors.New("payment amount must be greater than zero")
	ErrInvalidPaymentReference = errors.New("payment reference is required")
)

// PaymentMethod is the customer's payment preference (ADR 0006, section 3). It carries no card data.
type PaymentMethod string

const (
	PaymentCash PaymentMethod = "CASH"
	PaymentCard PaymentMethod = "CARD"
)

// ParsePaymentMethod accepts only the exact stored values.
func ParsePaymentMethod(raw string) (PaymentMethod, error) {
	switch m := PaymentMethod(raw); m {
	case PaymentCash, PaymentCard:
		return m, nil
	}
	return "", ErrInvalidPaymentMethod
}

// PaymentStatus is the outcome of a charge attempt. The MVP has no real gateway, so both values say
// "simulated" and the interface can never present them as a real charge (ADR 0006, section 4).
type PaymentStatus string

const (
	PaymentApproved PaymentStatus = "SIMULATED_APPROVED"
	PaymentDeclined PaymentStatus = "SIMULATED_DECLINED"
)

// PaymentAttempt is one row of the payment_attempts ledger. It has no field for card number, expiry
// or CVV: the system does not ask for, store or log them (ADR 0006, section 3).
type PaymentAttempt struct {
	ID         string
	CustomerID string
	Amount     decimal.Decimal
	Method     PaymentMethod
	Status     PaymentStatus
	Reference  string
	CreatedAt  time.Time
}

// NewPaymentAttempt validates a charge result. The amount is rounded to cents, like every money value.
func NewPaymentAttempt(customerID string, amount decimal.Decimal, method PaymentMethod, status PaymentStatus, reference string, at time.Time) (PaymentAttempt, error) {
	amount = amount.Round(2)
	if !amount.IsPositive() {
		return PaymentAttempt{}, ErrInvalidPaymentAmount
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return PaymentAttempt{}, ErrInvalidPaymentReference
	}
	return PaymentAttempt{CustomerID: customerID, Amount: amount, Method: method, Status: status, Reference: reference, CreatedAt: at.UTC()}, nil
}
