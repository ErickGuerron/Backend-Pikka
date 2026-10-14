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
