package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

var disputeNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func owed(createdAt time.Time) Surcharge {
	return Surcharge{
		ID: "s-1", CustomerID: "c-1", OrderID: "o-1", Reason: "CLIENT_REFUSED", Status: SurchargePending,
		Amount: decimal.NewNullDecimal(decimal.RequireFromString("4.20")), CreatedAt: createdAt,
	}
}

func TestNewDisputeInsideTheWindowKeepsTheCustomerReason(t *testing.T) {
	d, err := NewDispute(owed(disputeNow.Add(-90*time.Minute)), "  Yo estaba en casa y nadie tocó  ", disputeNow)

	if err != nil {
		t.Fatal(err)
	}
	if d.SurchargeID != "s-1" || d.CustomerID != "c-1" || d.Status != DisputeOpen || d.Reason != "Yo estaba en casa y nadie tocó" || !d.OpenedAt.Equal(disputeNow) {
		t.Fatalf("dispute = %+v", d)
	}
}

func TestTheWindowIsTwoHoursFromWhenTheSurchargeBecameOwed(t *testing.T) {
	if _, err := NewDispute(owed(disputeNow.Add(-DisputeWindow)), "Yo estaba en casa", disputeNow); err != nil {
		t.Fatalf("exactly at the limit must still be allowed: %v", err)
	}
	if _, err := NewDispute(owed(disputeNow.Add(-DisputeWindow-time.Second)), "Yo estaba en casa", disputeNow); !errors.Is(err, ErrDisputeWindowClosed) {
		t.Fatalf("err = %v, want ErrDisputeWindowClosed", err)
	}
}

func TestTheWindowStartsWhenTheOperatorConfirmedAWrongAddress(t *testing.T) {
	s := owed(disputeNow.Add(-48 * time.Hour))
	s.ResolvedAt = disputeNow.Add(-30 * time.Minute)

	if _, err := NewDispute(s, "La dirección era correcta", disputeNow); err != nil {
		t.Fatalf("the clock starts at the confirmation, not at the incident: %v", err)
	}
}

func TestOnlyAnOwedPricedSurchargeCanBeDisputed(t *testing.T) {
	cases := map[string]func(*Surcharge){
		"waiting for verification": func(s *Surcharge) { s.Status = SurchargePendingVerification },
		"already waived":           func(s *Surcharge) { s.Status = SurchargeWaived },
		"already paid":             func(s *Surcharge) { s.Status = SurchargePaid },
		"without an amount":        func(s *Surcharge) { s.Amount = decimal.NullDecimal{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := owed(disputeNow.Add(-time.Minute))
			mutate(&s)
			if _, err := NewDispute(s, "Yo estaba en casa", disputeNow); !errors.Is(err, ErrSurchargeNotDisputable) {
				t.Fatalf("err = %v, want ErrSurchargeNotDisputable", err)
			}
		})
	}
}

func TestADisputeNeedsAnAuditableReason(t *testing.T) {
	for _, reason := range []string{"", "ok", "      ", strings.Repeat("x", 301)} {
		if _, err := NewDispute(owed(disputeNow.Add(-time.Minute)), reason, disputeNow); !errors.Is(err, ErrReasonRequired) {
			t.Fatalf("reason %q: err = %v", reason, err)
		}
	}
}

func openDispute(t *testing.T) Dispute {
	t.Helper()
	d, err := NewDispute(owed(disputeNow.Add(-time.Minute)), "Yo estaba en casa", disputeNow)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolveClosesTheDisputeOnceWithTheOperatorAndAReason(t *testing.T) {
	for _, decision := range []DisputeStatus{DisputeInFavor, DisputeAgainst} {
		got, err := openDispute(t).Resolve(decision, "op-1", "  Revisamos la ruta y el timbre  ", disputeNow.Add(time.Hour))
		if err != nil {
			t.Fatalf("%s: %v", decision, err)
		}
		if got.Status != decision || got.ResolvedBy != "op-1" || got.ResolutionReason != "Revisamos la ruta y el timbre" || got.ResolvedAt.IsZero() {
			t.Fatalf("%s: dispute = %+v", decision, got)
		}
		if _, err := got.Resolve(decision, "op-2", "Otra vez lo mismo", disputeNow); !errors.Is(err, ErrDisputeAlreadyResolved) {
			t.Fatalf("%s: second resolve err = %v", decision, err)
		}
	}
}

func TestResolveRejectsWhatIsNotAuditableOrNotADecision(t *testing.T) {
	cases := map[string]struct {
		decision DisputeStatus
		operator string
		reason   string
		want     error
	}{
		"open is not a decision": {DisputeOpen, "op-1", "Revisamos la ruta", ErrUnknownDisputeDecision},
		"unknown decision":       {DisputeStatus("MAYBE"), "op-1", "Revisamos la ruta", ErrUnknownDisputeDecision},
		"no operator":            {DisputeInFavor, "", "Revisamos la ruta", ErrInvalidSurchargeIncident},
		"short reason":           {DisputeInFavor, "op-1", "ok", ErrReasonRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := openDispute(t)
			got, err := d.Resolve(tc.decision, tc.operator, tc.reason, disputeNow)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if got.Status != DisputeOpen {
				t.Fatalf("a rejected resolve must leave the dispute open, got %s", got.Status)
			}
		})
	}
}
