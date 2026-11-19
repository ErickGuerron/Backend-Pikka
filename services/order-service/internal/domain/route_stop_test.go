package domain

import (
	"errors"
	"testing"
)

// allStatuses lists every status so the transition table covers each source state.
var allStatuses = []Status{
	StatusPending, StatusPendingReview, StatusRouteAssigned, StatusPickedUp,
	StatusInTransit, StatusDelivered, StatusNotDelivered, StatusCancelled,
}

// Only ROUTE_ASSIGNED -> PICKED_UP and PICKED_UP -> IN_TRANSIT are legal driver steps.
func TestAdvanceRouteStopTransitionTable(t *testing.T) {
	legal := map[[2]Status]bool{
		{StatusRouteAssigned, StatusPickedUp}: true,
		{StatusPickedUp, StatusInTransit}:     true,
	}
	for _, from := range allStatuses {
		for _, target := range []Status{StatusPickedUp, StatusInTransit} {
			order := Order{Status: from, RouteID: "zone-1#1"}

			updated, err := order.AdvanceRouteStop(target)
			want := legal[[2]Status{from, target}]
			if want {
				if err != nil {
					t.Fatalf("%s -> %s: err = %v, want legal", from, target, err)
				}
				if updated.Status != target {
					t.Fatalf("%s -> %s: status = %s", from, target, updated.Status)
				}
				if updated.RouteID != "zone-1#1" {
					t.Fatalf("%s -> %s: route id lost: %q", from, target, updated.RouteID)
				}
				continue
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("%s -> %s: err = %v, want ErrInvalidTransition", from, target, err)
			}
			if updated.Status != from {
				t.Fatalf("%s -> %s: status changed to %s on rejection", from, target, updated.Status)
			}
		}
	}
}

// DELIVERED, NOT_DELIVERED and the non-route statuses are not driver steps here (RegisterDelivery owns them).
func TestAdvanceRouteStopRejectsTargetsOutsideDriverSteps(t *testing.T) {
	for _, from := range allStatuses {
		for _, target := range []Status{
			StatusPending, StatusPendingReview, StatusRouteAssigned,
			StatusDelivered, StatusNotDelivered, StatusCancelled, Status(""),
		} {
			order := Order{Status: from}

			_, err := order.AdvanceRouteStop(target)
			if !errors.Is(err, ErrInvalidAdvanceTarget) {
				t.Fatalf("%s -> target %q: err = %v, want ErrInvalidAdvanceTarget", from, target, err)
			}
		}
	}
}

func TestIsRouteStopTarget(t *testing.T) {
	if !IsRouteStopTarget(StatusPickedUp) || !IsRouteStopTarget(StatusInTransit) {
		t.Fatal("PICKED_UP and IN_TRANSIT must be route stop targets")
	}
	for _, s := range []Status{StatusDelivered, StatusNotDelivered, StatusRouteAssigned, Status("")} {
		if IsRouteStopTarget(s) {
			t.Fatalf("%q must not be a route stop target", s)
		}
	}
}
