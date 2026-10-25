package domain

import "testing"

// Only orders that hold a driver route slot count against the zone capacity. PENDING orders
// are waiting, and closed or cancelled orders have released their slot.
func TestOccupiesRouteSlot(t *testing.T) {
	cases := map[Status]bool{
		StatusPending:       false,
		StatusPendingReview: false,
		StatusRouteAssigned: true,
		StatusPickedUp:      true,
		StatusInTransit:     true,
		StatusDelivered:     false,
		StatusNotDelivered:  false,
		StatusCancelled:     false,
	}
	for status, want := range cases {
		if got := status.OccupiesRouteSlot(); got != want {
			t.Fatalf("%s.OccupiesRouteSlot() = %v, want %v", status, got, want)
		}
	}
}
