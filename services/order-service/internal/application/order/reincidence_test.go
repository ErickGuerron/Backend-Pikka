package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type reincidenceRepoFake struct {
	ports.SurchargeRepository
	times       []time.Time
	timesErr    error
	marked      []string
	alreadyMark bool
	listCalls   int
}

func (f *reincidenceRepoFake) CountedIncidentTimes(_ context.Context, _ string) ([]time.Time, error) {
	f.listCalls++
	return f.times, f.timesErr
}

func (f *reincidenceRepoFake) MarkCustomerRecidivist(_ context.Context, customerID string, _ time.Time) (bool, error) {
	if f.alreadyMark {
		return false, nil
	}
	f.marked = append(f.marked, customerID)
	return true, nil
}

func at(days ...int) []time.Time {
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	out := []time.Time{}
	for _, d := range days {
		out = append(out, base.AddDate(0, 0, d))
	}
	return out
}

func TestReincidenceEvaluateMarksTheCustomerOnTheThird(t *testing.T) {
	repo := &reincidenceRepoFake{times: at(0, 4, 9)}

	marked, err := NewReincidence(repo).Evaluate(context.Background(), "cust-1")

	if err != nil || !marked {
		t.Fatalf("marked = %v, err = %v", marked, err)
	}
	if len(repo.marked) != 1 || repo.marked[0] != "cust-1" {
		t.Fatalf("marked customers = %v", repo.marked)
	}
}

func TestReincidenceEvaluateDoesNotMarkBelowThreshold(t *testing.T) {
	repo := &reincidenceRepoFake{times: at(0, 4)}

	marked, err := NewReincidence(repo).Evaluate(context.Background(), "cust-1")

	if err != nil || marked || len(repo.marked) != 0 {
		t.Fatalf("marked = %v, err = %v, customers = %v", marked, err, repo.marked)
	}
}

// A customer already marked is not marked again: the first mark keeps its date.
func TestReincidenceEvaluateIsIdempotent(t *testing.T) {
	repo := &reincidenceRepoFake{times: at(0, 1, 2), alreadyMark: true}

	marked, err := NewReincidence(repo).Evaluate(context.Background(), "cust-1")

	if err != nil || marked {
		t.Fatalf("marked = %v, err = %v", marked, err)
	}
}

func TestReincidenceEvaluateReturnsRepositoryErrors(t *testing.T) {
	repo := &reincidenceRepoFake{timesErr: errors.New("db down")}
	if _, err := NewReincidence(repo).Evaluate(context.Background(), "cust-1"); err == nil {
		t.Fatal("want the repository error")
	}
}

// --- hooks ---

func TestNewClientIncidentEvaluatesReincidence(t *testing.T) {
	surcharges := &surchargeRepoFake{}
	reinc := &reincidenceRepoFake{times: at(0, 1, 2)}
	id := uuid.NewString()
	repo := &outcomeRepo{current: domain.Order{ID: id, CustomerID: "cust-1", Status: domain.StatusInTransit, RouteID: "z#1"}}

	_, err := NewRecordStopOutcome(repo).WithSurcharges(surcharges).WithReincidence(NewReincidence(reinc)).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: domain.StatusNotDelivered, Reason: "CLIENT_REFUSED",
	})

	if err != nil {
		t.Fatal(err)
	}
	if len(reinc.marked) != 1 {
		t.Fatalf("customer must be marked, got %v", reinc.marked)
	}
}

// A WRONG_ADDRESS incident waits for the operator, so it does not count yet.
func TestWrongAddressIncidentDoesNotEvaluateReincidence(t *testing.T) {
	reinc := &reincidenceRepoFake{times: at(0, 1, 2)}
	id := uuid.NewString()
	repo := &outcomeRepo{current: domain.Order{ID: id, CustomerID: "cust-1", Status: domain.StatusInTransit, RouteID: "z#1"}}

	_, err := NewRecordStopOutcome(repo).WithSurcharges(&surchargeRepoFake{}).WithReincidence(NewReincidence(reinc)).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: domain.StatusNotDelivered, Reason: "WRONG_ADDRESS",
	})

	if err != nil {
		t.Fatal(err)
	}
	if reinc.listCalls != 0 {
		t.Fatalf("reincidence evaluated %d times, want 0", reinc.listCalls)
	}
}

func TestReincidenceFailureDoesNotFailTheStop(t *testing.T) {
	reinc := &reincidenceRepoFake{timesErr: errors.New("db down")}
	id := uuid.NewString()
	repo := &outcomeRepo{current: domain.Order{ID: id, CustomerID: "cust-1", Status: domain.StatusInTransit, RouteID: "z#1"}}

	order, err := NewRecordStopOutcome(repo).WithSurcharges(&surchargeRepoFake{}).WithReincidence(NewReincidence(reinc)).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: domain.StatusNotDelivered, Reason: "CLIENT_ABSENT",
	})

	if err != nil || order.Status != domain.StatusNotDelivered {
		t.Fatalf("status = %s, err = %v", order.Status, err)
	}
}

// A confirmed WRONG_ADDRESS becomes a customer incident, so it counts from that moment.
func TestConfirmedVerificationEvaluatesReincidenceButWaivedDoesNot(t *testing.T) {
	for decision, wantCalls := range map[string]int{"CONFIRM": 1, "WAIVE": 0} {
		repo := &verificationRepoFake{stored: waitingSurcharge()}
		reinc := &reincidenceRepoFake{times: at(0, 1, 2)}
		uc := NewResolveSurchargeVerification(repo).WithReincidence(NewReincidence(reinc))

		_, err := uc.Run(context.Background(), ResolveSurchargeCommand{
			Role: RoleOperator, OperatorID: "op-1", SurchargeID: repo.stored.ID, Decision: decision, Reason: "Verificado con el cliente",
		})

		if err != nil {
			t.Fatalf("%s: %v", decision, err)
		}
		if reinc.listCalls != wantCalls {
			t.Fatalf("%s: evaluations = %d, want %d", decision, reinc.listCalls, wantCalls)
		}
	}
}
