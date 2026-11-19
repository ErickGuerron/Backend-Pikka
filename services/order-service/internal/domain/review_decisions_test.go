package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// inReview devuelve un pedido en PENDING_REVIEW con los motivos indicados.
func inReview(t *testing.T, reasons ...domain.ReviewReason) domain.Order {
	t.Helper()
	order, err := pendingOrder(t).ApplyValidation(reasons)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if order.Status != domain.StatusPendingReview {
		t.Fatalf("setup: status = %s, want PENDING_REVIEW", order.Status)
	}
	return order
}

func TestReviewReasonSpatialOnlyForWhereTheOrderGoes(t *testing.T) {
	tests := map[domain.ReviewReason]bool{
		domain.ReviewZoneNotCovered:    true,
		domain.ReviewAddressNotLocated: true,
		domain.ReviewInvalidWindow:     false,
		domain.ReviewCapacityExceeded:  false,
		domain.ReviewCustomerBlocked:   false,
	}
	for reason, want := range tests {
		t.Run(string(reason), func(t *testing.T) {
			if got := reason.Spatial(); got != want {
				t.Fatalf("Spatial() = %v, want %v", got, want)
			}
		})
	}
}

func TestConfirmAcceptsNonSpatialReviewAndClearsReasons(t *testing.T) {
	order, err := inReview(t, domain.ReviewCustomerBlocked, domain.ReviewCapacityExceeded).Confirm()
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if order.Status != domain.StatusPending || len(order.ReviewReasons) != 0 {
		t.Fatalf("want PENDING without reasons, got %s %v", order.Status, order.ReviewReasons)
	}
}

func TestConfirmBlockedBySpatialReason(t *testing.T) {
	_, err := inReview(t, domain.ReviewCustomerBlocked, domain.ReviewZoneNotCovered).Confirm()
	if !errors.Is(err, domain.ErrSpatialReasonNotOverridable) {
		t.Fatalf("err = %v, want ErrSpatialReasonNotOverridable", err)
	}
}

func TestConfirmRequiresReviewStatus(t *testing.T) {
	_, err := pendingOrder(t).Confirm()
	if !errors.Is(err, domain.ErrOrderNotInReview) {
		t.Fatalf("err = %v, want ErrOrderNotInReview", err)
	}
}

func TestRejectCancelsOrderInReview(t *testing.T) {
	order, err := inReview(t, domain.ReviewZoneNotCovered).Reject()
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if order.Status != domain.StatusCancelled || len(order.ReviewReasons) != 0 {
		t.Fatalf("want CANCELLED without reasons, got %s %v", order.Status, order.ReviewReasons)
	}
}

func TestRejectRequiresReviewStatus(t *testing.T) {
	_, err := pendingOrder(t).Reject()
	if !errors.Is(err, domain.ErrOrderNotInReview) {
		t.Fatalf("err = %v, want ErrOrderNotInReview", err)
	}
}

func TestNotificationDetailIsBoundedToTableLimitInRunes(t *testing.T) {
	long := strings.Repeat("ñ", 350)

	n := domain.NewNotification("cust", "order", domain.NotificationRejected, long)

	if got := len([]rune(n.Detail)); got != 300 {
		t.Fatalf("detail runes = %d, want 300", got)
	}
}

func TestNotificationDetailShorterThanLimitIsKept(t *testing.T) {
	n := domain.NewNotification("cust", "order", domain.NotificationRejected, "motivo corto")

	if n.Detail != "motivo corto" {
		t.Fatalf("detail = %q", n.Detail)
	}
}
