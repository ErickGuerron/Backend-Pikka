package domain

import (
	"errors"
	"testing"
)

func TestCancelByCustomerAllowedFromPendingAndRouteAssigned(t *testing.T) {
	for _, from := range []Status{StatusPending, StatusRouteAssigned} {
		order := Order{ID: "o-1", Status: from}

		got, err := order.CancelByCustomer()
		if err != nil {
			t.Fatalf("from %s: CancelByCustomer: %v", from, err)
		}
		if got.Status != StatusCancelled {
			t.Fatalf("from %s: status = %s, want CANCELLED", from, got.Status)
		}
	}
}

func TestCancelByCustomerRejectedOutsidePendingAndRouteAssigned(t *testing.T) {
	for _, from := range []Status{
		StatusPendingReview, // el cliente no cancela mientras el operador revisa
		StatusPickedUp,
		StatusInTransit,
		StatusDelivered,
		StatusNotDelivered,
		StatusCancelled,
	} {
		order := Order{ID: "o-1", Status: from}

		got, err := order.CancelByCustomer()
		if !errors.Is(err, ErrOrderNotCancellable) {
			t.Fatalf("from %s: err = %v, want ErrOrderNotCancellable", from, err)
		}
		if got.Status != from {
			t.Fatalf("from %s: a rejected cancellation must not change the order, got %s", from, got.Status)
		}
	}
}
