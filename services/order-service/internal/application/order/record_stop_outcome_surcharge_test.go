package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type surchargeRepoFake struct {
	ports.SurchargeRepository
	recorded []domain.Surcharge
	err      error
}

var _ ports.SurchargeRepository = (*surchargeRepoFake)(nil)

func (f *surchargeRepoFake) RecordSurchargeIncident(_ context.Context, s domain.Surcharge) (domain.Surcharge, bool, error) {
	if f.err != nil {
		return domain.Surcharge{}, false, f.err
	}
	f.recorded = append(f.recorded, s)
	return s, true, nil
}

func runOutcome(t *testing.T, outcome domain.Status, reason string, surcharges *surchargeRepoFake) (domain.Order, error) {
	t.Helper()
	id := uuid.NewString()
	repo := &outcomeRepo{current: domain.Order{ID: id, CustomerID: "cust-1", Status: domain.StatusInTransit, RouteID: "z#1"}}
	return NewRecordStopOutcome(repo).WithSurcharges(surcharges).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: outcome, Reason: reason,
	})
}

func TestNotDeliveredByClientRecordsSurchargeIncident(t *testing.T) {
	for _, reason := range []string{"CLIENT_REFUSED", "CLIENT_ABSENT", "OUT_OF_WINDOW"} {
		surcharges := &surchargeRepoFake{}
		order, err := runOutcome(t, domain.StatusNotDelivered, reason, surcharges)
		if err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if len(surcharges.recorded) != 1 {
			t.Fatalf("%s: incidents = %d, want 1", reason, len(surcharges.recorded))
		}
		got := surcharges.recorded[0]
		if got.CustomerID != "cust-1" || got.OrderID != order.ID || got.Reason != reason || got.Status != domain.SurchargePending {
			t.Fatalf("%s: incident = %+v", reason, got)
		}
	}
}

func TestNoSurchargeWhenNothingChargeable(t *testing.T) {
	cases := []struct {
		name    string
		outcome domain.Status
		reason  string
	}{
		{"delivered", domain.StatusDelivered, ""},
		{"delivered with stray reason", domain.StatusDelivered, "CLIENT_REFUSED"},
		{"business reason", domain.StatusNotDelivered, "PRODUCT_DAMAGED"},
		{"duplicate order", domain.StatusNotDelivered, "DUPLICATE_ORDER"},
		{"missing reason from an old caller", domain.StatusNotDelivered, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			surcharges := &surchargeRepoFake{}
			if _, err := runOutcome(t, tc.outcome, tc.reason, surcharges); err != nil {
				t.Fatal(err)
			}
			if len(surcharges.recorded) != 0 {
				t.Fatalf("incidents = %d, want 0", len(surcharges.recorded))
			}
		})
	}
}

// ADR 0006 amendment: a wrong address is never charged automatically; it waits for the operator.
func TestWrongAddressRecordsIncidentPendingVerification(t *testing.T) {
	surcharges := &surchargeRepoFake{}
	order, err := runOutcome(t, domain.StatusNotDelivered, "WRONG_ADDRESS", surcharges)
	if err != nil {
		t.Fatal(err)
	}
	if len(surcharges.recorded) != 1 {
		t.Fatalf("incidents = %d, want 1", len(surcharges.recorded))
	}
	got := surcharges.recorded[0]
	if got.Status != domain.SurchargePendingVerification || got.OrderID != order.ID || got.Reason != "WRONG_ADDRESS" {
		t.Fatalf("incident = %+v", got)
	}
}

// The stop is already closed when the incident is recorded: a failure is logged, not returned, and
// the unique order_id makes a later retry safe.
func TestSurchargeFailureDoesNotUndoStopClosure(t *testing.T) {
	surcharges := &surchargeRepoFake{err: errors.New("db down")}
	order, err := runOutcome(t, domain.StatusNotDelivered, "CLIENT_REFUSED", surcharges)
	if err != nil {
		t.Fatalf("closing the stop must not fail: %v", err)
	}
	if order.Status != domain.StatusNotDelivered {
		t.Fatalf("status = %s", order.Status)
	}
}

// Existing behavior: without surcharges wired (and without reason) the use case works as before.
func TestRecordStopOutcomeWithoutSurchargesStillCloses(t *testing.T) {
	id := uuid.NewString()
	repo := &outcomeRepo{current: domain.Order{ID: id, Status: domain.StatusInTransit, RouteID: "z#1"}}
	got, err := NewRecordStopOutcome(repo).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: domain.StatusNotDelivered, Reason: "CLIENT_REFUSED",
	})
	if err != nil || got.Status != domain.StatusNotDelivered {
		t.Fatalf("got %v, %v", got.Status, err)
	}
}
