package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// NotificationKind is the event a customer is told about (ADR 0007, section 3).
type NotificationKind string

const (
	NotificationInReview  NotificationKind = "ORDER_IN_REVIEW"
	NotificationConfirmed NotificationKind = "ORDER_CONFIRMED"
	NotificationCorrected NotificationKind = "ORDER_CORRECTED"
	NotificationRejected  NotificationKind = "ORDER_REJECTED"

	// NotificationSurchargeApplied tells the customer a surcharge is owed (ADR 0006, section 5). Its detail is
	// "REASON,AMOUNT,DUE" with a closed reason code, a decimal amount and the RFC 3339 due date: no language, so each
	// app translates it.
	NotificationSurchargeApplied NotificationKind = "SURCHARGE_APPLIED"
	// NotificationAccountBlocked tells the customer the account was blocked for an unpaid balance. It belongs to the
	// account, not to an order, so it has no order and no detail.
	NotificationAccountBlocked NotificationKind = "ACCOUNT_BLOCKED_BY_BALANCE"
)

// Notification is a notice stored for the customer who owns the order.
type Notification struct {
	ID         string
	CustomerID string
	OrderID    string
	Kind       NotificationKind
	Detail     string
	CreatedAt  time.Time
	Read       bool
}

// NewNotification builds a notice for an order. Detail is trimmed and bounded
// to the table limit (300 characters).
func NewNotification(customerID, orderID string, kind NotificationKind, detail string) Notification {
	detail = trimToRunes(detail, 300)
	return Notification{CustomerID: customerID, OrderID: orderID, Kind: kind, Detail: detail}
}

// ReviewDecisionReason validates the operator's audit reason: 5 to 300 characters after trimming.
func ReviewDecisionReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	length := utf8.RuneCountInString(trimmed)
	if length < 5 || length > 300 {
		return "", ErrReasonRequired
	}
	return trimmed, nil
}

func trimToRunes(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

// NewSurchargeAppliedNotification builds the notice for a surcharge that is owed now: PENDING and priced. A surcharge
// waiting for the operator (PENDING_VERIFICATION) is not notified as definitive, and one without amount, waived or paid
// has nothing to pay, so ok is false for all of them.
func NewSurchargeAppliedNotification(s Surcharge) (Notification, bool) {
	if s.Status != SurchargePending || !s.Amount.Valid {
		return Notification{}, false
	}
	detail := strings.Join([]string{s.Reason, s.Amount.Decimal.StringFixed(2), s.DueAt().UTC().Format(time.RFC3339)}, ",")
	return NewNotification(s.CustomerID, s.OrderID, NotificationSurchargeApplied, detail), true
}

// NewAccountBlockedNotification builds the notice that the account was blocked for an unpaid balance.
func NewAccountBlockedNotification(customerID string) Notification {
	return NewNotification(customerID, "", NotificationAccountBlocked, "")
}
