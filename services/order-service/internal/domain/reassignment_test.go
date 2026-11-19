package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestNewReassignmentFeeRoundsToCentsAndKeepsTheAudit(t *testing.T) {
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.FixedZone("x", -5*3600))

	fee, err := NewReassignmentFee(decimal.RequireFromString("2.499"), "a-1", "  Sube la gasolina  ", at)

	if err != nil {
		t.Fatal(err)
	}
	if fee.Amount.StringFixed(2) != "2.50" || fee.SetBy != "a-1" || fee.Reason != "Sube la gasolina" || fee.CreatedAt.Location() != time.UTC {
		t.Fatalf("fee = %+v", fee)
	}
}

func TestNewReassignmentFeeRejectsWhatIsNotAnAuditedPositiveAmount(t *testing.T) {
	cases := map[string]struct {
		amount string
		admin  string
		reason string
		want   error
	}{
		"zero":             {"0", "a-1", "Sube la gasolina", ErrInvalidReassignmentFee},
		"rounds to zero":   {"0.004", "a-1", "Sube la gasolina", ErrInvalidReassignmentFee},
		"negative":         {"-1", "a-1", "Sube la gasolina", ErrInvalidReassignmentFee},
		"no administrator": {"2", "", "Sube la gasolina", ErrInvalidReassignmentFee},
		"short reason":     {"2", "a-1", "ok", ErrReasonRequired},
		"long reason":      {"2", "a-1", strings.Repeat("x", 301), ErrReasonRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewReassignmentFee(decimal.RequireFromString(tc.amount), tc.admin, tc.reason, time.Now()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewReassignmentChargeUsesTheFeeAmountAndNeverInventsOne(t *testing.T) {
	fee := &ReassignmentFee{Amount: decimal.RequireFromString("2.50")}

	got, err := NewReassignmentCharge("c-1", "o-1", fee, "op-1", "  Pidió cambiar la entrega  ", time.Now())

	if err != nil {
		t.Fatal(err)
	}
	if got.Amount.StringFixed(2) != "2.50" || got.CustomerID != "c-1" || got.OrderID != "o-1" || got.AppliedBy != "op-1" || got.Reason != "Pidió cambiar la entrega" {
		t.Fatalf("charge = %+v", got)
	}
	if _, err := NewReassignmentCharge("c-1", "o-1", nil, "op-1", "Pidió cambiar la entrega", time.Now()); !errors.Is(err, ErrReassignmentFeeNotSet) {
		t.Fatalf("no fee err = %v", err)
	}
}

func TestNewReassignmentChargeRejectsIncompleteDecisions(t *testing.T) {
	fee := &ReassignmentFee{Amount: decimal.RequireFromString("2.50")}
	cases := map[string]struct {
		customer, order, operator, reason string
		want                              error
	}{
		"no customer":  {"", "o-1", "op-1", "Pidió cambiar la entrega", ErrInvalidReassignmentCharge},
		"no order":     {"c-1", "", "op-1", "Pidió cambiar la entrega", ErrInvalidReassignmentCharge},
		"no operator":  {"c-1", "o-1", "", "Pidió cambiar la entrega", ErrInvalidReassignmentCharge},
		"short reason": {"c-1", "o-1", "op-1", "ok", ErrReasonRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewReassignmentCharge(tc.customer, tc.order, fee, tc.operator, tc.reason, time.Now()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
