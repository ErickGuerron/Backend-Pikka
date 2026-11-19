package domain

import (
	"errors"
	"testing"
)

func TestAssignToRouteStoresRouteAndStopSequenceAndAllowsOnlyPending(t *testing.T) {
	order := Order{Status: StatusPending}

	updated, err := order.AssignToRoute("zone-1#1", 3)
	if err != nil {
		t.Fatalf("AssignToRoute from PENDING: %v", err)
	}
	if updated.Status != StatusRouteAssigned || updated.RouteID != "zone-1#1" || updated.StopSequence != 3 {
		t.Fatalf("status = %s route = %q sequence = %d, want ROUTE_ASSIGNED zone-1#1 3", updated.Status, updated.RouteID, updated.StopSequence)
	}
	if order.Status != StatusPending || order.RouteID != "" || order.StopSequence != 0 {
		t.Fatalf("original order mutated: %s %q %d", order.Status, order.RouteID, order.StopSequence)
	}
}

// Sequence 0 is the legacy "unassigned" value (order_ids form). Negative values are never valid.
func TestAssignToRouteSequenceZeroIsUnassignedAndNegativeIsRejected(t *testing.T) {
	updated, err := Order{Status: StatusPending}.AssignToRoute("zone-1#1", 0)
	if err != nil {
		t.Fatalf("sequence 0: %v", err)
	}
	if updated.StopSequence != 0 {
		t.Fatalf("sequence = %d, want 0 (unassigned)", updated.StopSequence)
	}

	for _, sequence := range []int{-1, -50} {
		order := Order{Status: StatusPending}

		_, err := order.AssignToRoute("zone-1#1", sequence)
		if !errors.Is(err, ErrInvalidStopSequence) {
			t.Fatalf("sequence %d: err = %v, want ErrInvalidStopSequence", sequence, err)
		}
	}
}

func TestAssignToRouteRejectsBlankRoute(t *testing.T) {
	for _, route := range []string{"", "   "} {
		order := Order{Status: StatusPending}

		_, err := order.AssignToRoute(route, 1)
		if !errors.Is(err, ErrInvalidRouteID) {
			t.Fatalf("route %q: err = %v, want ErrInvalidRouteID", route, err)
		}
	}
}

func TestAssignToRouteRejectsEveryOtherStatus(t *testing.T) {
	for _, from := range []Status{
		StatusPendingReview, StatusRouteAssigned, StatusPickedUp,
		StatusInTransit, StatusDelivered, StatusNotDelivered, StatusCancelled,
	} {
		order := Order{Status: from}

		updated, err := order.AssignToRoute("zone-1#1", 1)
		if !errors.Is(err, ErrOrderNotPending) {
			t.Fatalf("from %s: err = %v, want ErrOrderNotPending", from, err)
		}
		if updated.Status != from {
			t.Fatalf("from %s: status changed to %s on rejection", from, updated.Status)
		}
	}
}
