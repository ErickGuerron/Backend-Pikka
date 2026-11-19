package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewRecidivistClearKeepsTheAuditedDecision(t *testing.T) {
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.FixedZone("x", -5*3600))
	marked := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

	got, err := NewRecidivistClear("c-1", marked, "a-1", "  Pagó todo y se acordó con él  ", at)

	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerID != "c-1" || got.ClearedBy != "a-1" || got.Reason != "Pagó todo y se acordó con él" ||
		!got.RecidivistAt.Equal(marked) || got.CreatedAt.Location() != time.UTC || !got.CreatedAt.Equal(at) {
		t.Fatalf("clear = %+v", got)
	}
}

func TestNewRecidivistClearRejectsWhatIsNotAuditable(t *testing.T) {
	cases := map[string]struct {
		customer, admin, reason string
		want                    error
	}{
		"no customer":  {"", "a-1", "Pagó todo y se acordó", ErrInvalidRecidivistClear},
		"no admin":     {"c-1", "", "Pagó todo y se acordó", ErrInvalidRecidivistClear},
		"short reason": {"c-1", "a-1", "ok", ErrReasonRequired},
		"blank reason": {"c-1", "a-1", "      ", ErrReasonRequired},
		"long reason":  {"c-1", "a-1", strings.Repeat("x", 301), ErrReasonRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRecidivistClear(tc.customer, time.Time{}, tc.admin, tc.reason, time.Now()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
