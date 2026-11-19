package domain

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrReassignmentFeeNotSet     = errors.New("the reassignment fee has not been set by an administrator")
	ErrInvalidReassignmentFee    = errors.New("reassignment fee must be an amount greater than zero")
	ErrInvalidReassignmentCharge = errors.New("reassignment charge needs a customer, an order and an operator")
	ErrReassignmentChargeExists  = errors.New("a reassignment charge was already applied to this order")
)

// ReassignmentFee is the fixed amount charged when an order is reassigned because the customer asked for it
// with the driver already on the way (ADR 0006, wrong address). An administrator sets it and may change it, with
// an audited reason; every change is a new row, and the latest one is the current fee.
type ReassignmentFee struct {
	ID        string
	Amount    decimal.Decimal
	Reason    string
	SetBy     string
	CreatedAt time.Time
}

// NewReassignmentFee validates the administrator's decision: a positive amount in cents and a reason of 5 to 300 characters.
func NewReassignmentFee(amount decimal.Decimal, adminID, reason string, at time.Time) (ReassignmentFee, error) {
	amount = amount.Round(2)
	if adminID == "" || !amount.IsPositive() {
		return ReassignmentFee{}, ErrInvalidReassignmentFee
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return ReassignmentFee{}, err
	}
	return ReassignmentFee{Amount: amount, Reason: reason, SetBy: adminID, CreatedAt: at.UTC()}, nil
}

// ReassignmentCharge is the fee applied to one order. It is unique per order, so a retry never charges twice.
type ReassignmentCharge struct {
	ID         string
	CustomerID string
	OrderID    string
	Amount     decimal.Decimal
	Reason     string
	AppliedBy  string
	CreatedAt  time.Time
}

// NewReassignmentCharge charges the current fee. With no fee set there is nothing to charge: the amount is never invented.
func NewReassignmentCharge(customerID, orderID string, fee *ReassignmentFee, operatorID, reason string, at time.Time) (ReassignmentCharge, error) {
	if fee == nil {
		return ReassignmentCharge{}, ErrReassignmentFeeNotSet
	}
	if customerID == "" || orderID == "" || operatorID == "" {
		return ReassignmentCharge{}, ErrInvalidReassignmentCharge
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return ReassignmentCharge{}, err
	}
	return ReassignmentCharge{
		CustomerID: customerID, OrderID: orderID, Amount: fee.Amount, Reason: reason, AppliedBy: operatorID, CreatedAt: at.UTC(),
	}, nil
}
