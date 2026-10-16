package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func pendingOrder(t *testing.T) domain.Order {
	t.Helper()
	order, err := domain.NewOrder(domain.NewOrderInput{
		Address:      "Avenida Amazonas 123",
		Latitude:     -0.18,
		Longitude:    -78.48,
		DeliveryDate: "2030-04-12",
		WindowStart:  "09:00",
		WindowEnd:    "12:00",

		AddressReference: "Casa azul", ContactPhone: "0991234567",
	}, time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return order
}

func TestApplyValidationWithoutReasonsKeepsOrderPending(t *testing.T) {
	order, err := pendingOrder(t).ApplyValidation(nil)
	if err != nil {
		t.Fatalf("ApplyValidation: %v", err)
	}
	if order.Status != domain.StatusPending || len(order.ReviewReasons) != 0 {
		t.Fatalf("want PENDING without reasons, got %s %v", order.Status, order.ReviewReasons)
	}
}

func TestApplyValidationWithReasonsMovesOrderToReview(t *testing.T) {
	reasons := []domain.ReviewReason{domain.ReviewZoneNotCovered, domain.ReviewCustomerBlocked}
	order, err := pendingOrder(t).ApplyValidation(reasons)
	if err != nil {
		t.Fatalf("ApplyValidation: %v", err)
	}
	if order.Status != domain.StatusPendingReview {
		t.Fatalf("status = %s, want PENDING_REVIEW", order.Status)
	}
	if len(order.ReviewReasons) != 2 || order.ReviewReasons[0] != domain.ReviewZoneNotCovered {
		t.Fatalf("reasons = %v", order.ReviewReasons)
	}
}

func TestApplyValidationRejectsUnknownReason(t *testing.T) {
	_, err := pendingOrder(t).ApplyValidation([]domain.ReviewReason{"MADE_UP"})
	if !errors.Is(err, domain.ErrInvalidReviewReason) {
		t.Fatalf("err = %v, want ErrInvalidReviewReason", err)
	}
}

func TestApplyValidationOnlyAppliesToPendingOrders(t *testing.T) {
	order, err := pendingOrder(t).ApplyValidation([]domain.ReviewReason{domain.ReviewZoneNotCovered})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := order.ApplyValidation(nil); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}
