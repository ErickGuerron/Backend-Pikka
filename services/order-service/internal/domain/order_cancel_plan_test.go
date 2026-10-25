package domain

import (
	"errors"
	"testing"
)

var planAllStatuses = []Status{
	StatusPending, StatusPendingReview, StatusRouteAssigned, StatusPickedUp,
	StatusInTransit, StatusDelivered, StatusNotDelivered, StatusCancelled,
}

// Plan: cancelación de pedidos en todos los estados.
func TestPlan_Order_CancelAcrossAllStatuses(t *testing.T) {
	tests := []struct {
		from      Status
		cancelOK  bool
		wantAfter Status
	}{
		{StatusPending, true, StatusCancelled},
		{StatusPendingReview, false, StatusPendingReview},
		{StatusRouteAssigned, true, StatusCancelled},
		{StatusPickedUp, false, StatusPickedUp},
		{StatusInTransit, false, StatusInTransit},
		{StatusDelivered, false, StatusDelivered},
		{StatusNotDelivered, false, StatusNotDelivered},
		{StatusCancelled, false, StatusCancelled},
	}
	if len(tests) != len(planAllStatuses) {
		t.Fatalf("table covers %d statuses, want %d", len(tests), len(planAllStatuses))
	}
	for _, tc := range tests {
		t.Run(string(tc.from), func(t *testing.T) {
			if got := tc.from.CanCancel(); got != tc.cancelOK {
				t.Fatalf("CanCancel = %v, want %v", got, tc.cancelOK)
			}
			got, err := Order{ID: "o-1", Status: tc.from}.CancelByCustomer()
			if tc.cancelOK && err != nil {
				t.Fatalf("CancelByCustomer: %v", err)
			}
			if !tc.cancelOK && !errors.Is(err, ErrOrderNotCancellable) {
				t.Fatalf("err = %v, want ErrOrderNotCancellable", err)
			}
			if got.Status != tc.wantAfter {
				t.Fatalf("status = %s, want %s", got.Status, tc.wantAfter)
			}
		})
	}
}

func TestPlan_Order_CancelledCannotTransitionToAnyStatus(t *testing.T) {
	for _, next := range planAllStatuses {
		t.Run(string(next), func(t *testing.T) {
			if StatusCancelled.CanTransitionTo(next) {
				t.Fatalf("CANCELLED -> %s must be rejected", next)
			}
			got, err := Order{ID: "o-1", Status: StatusCancelled}.TransitionTo(next)
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("err = %v, want ErrInvalidTransition", err)
			}
			if got.Status == next {
				t.Fatalf("rejected transition must not apply %s", next)
			}
		})
	}
}
