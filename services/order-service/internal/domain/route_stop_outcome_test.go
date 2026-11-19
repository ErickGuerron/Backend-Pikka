package domain

import (
	"errors"
	"testing"
)

// Only IN_TRANSIT -> DELIVERED and IN_TRANSIT -> NOT_DELIVERED close a stop of a route.
func TestRecordStopOutcomeTransitionTable(t *testing.T) {
	for _, from := range allStatuses {
		for _, outcome := range []Status{StatusDelivered, StatusNotDelivered} {
			order := Order{Status: from, RouteID: "zone-1#1"}

			updated, err := order.RecordStopOutcome(outcome)
			if from == StatusInTransit {
				if err != nil {
					t.Fatalf("%s -> %s: err = %v, want legal", from, outcome, err)
				}
				if updated.Status != outcome {
					t.Fatalf("%s -> %s: status = %s", from, outcome, updated.Status)
				}
				if updated.RouteID != "zone-1#1" {
					t.Fatalf("%s -> %s: route id lost: %q", from, outcome, updated.RouteID)
				}
				continue
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("%s -> %s: err = %v, want ErrInvalidTransition", from, outcome, err)
			}
			if updated.Status != from {
				t.Fatalf("%s -> %s: status changed to %s on rejection", from, outcome, updated.Status)
			}
		}
	}
}

// Any status other than DELIVERED or NOT_DELIVERED is not an outcome.
func TestRecordStopOutcomeRejectsNonOutcomes(t *testing.T) {
	for _, from := range allStatuses {
		for _, target := range []Status{
			StatusPending, StatusPendingReview, StatusRouteAssigned, StatusPickedUp,
			StatusInTransit, StatusCancelled, Status(""),
		} {
			order := Order{Status: from}

			_, err := order.RecordStopOutcome(target)
			if !errors.Is(err, ErrInvalidStopOutcome) {
				t.Fatalf("%s -> outcome %q: err = %v, want ErrInvalidStopOutcome", from, target, err)
			}
		}
	}
}
