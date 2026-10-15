package domain

import "testing"

func TestAssignToRouteStoresRouteAndAllowsOnlyPending(t *testing.T) {
	order := Order{Status: StatusPending}

	updated, err := order.AssignToRoute("zone-1#1")
	if err != nil {
		t.Fatalf("AssignToRoute from PENDING: %v", err)
	}
	if updated.Status != StatusRouteAssigned || updated.RouteID != "zone-1#1" {
		t.Fatalf("status = %s route = %q, want ROUTE_ASSIGNED zone-1#1", updated.Status, updated.RouteID)
	}
	if order.Status != StatusPending || order.RouteID != "" {
		t.Fatalf("original order mutated: %s %q", order.Status, order.RouteID)
	}
}

func TestAssignToRouteRejectsBlankRoute(t *testing.T) {
	for _, route := range []string{"", "   "} {
		order := Order{Status: StatusPending}

		_, err := order.AssignToRoute(route)
		if err != ErrInvalidRouteID {
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

		updated, err := order.AssignToRoute("zone-1#1")
		if err != ErrOrderNotPending {
			t.Fatalf("from %s: err = %v, want ErrOrderNotPending", from, err)
		}
		if updated.Status != from {
			t.Fatalf("from %s: status changed to %s on rejection", from, updated.Status)
		}
	}
}
