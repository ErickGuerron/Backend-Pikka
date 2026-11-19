package domain

import "errors"

// ErrInvalidReviewReason is returned when a reason outside the closed list is applied.
var ErrInvalidReviewReason = errors.New("unknown review reason")

// ReviewReason is a business rule that the automatic validation (ADR 0007, section 3)
// could not satisfy. The set is closed and mirrors the orders table constraint.
type ReviewReason string

const (
	ReviewZoneNotCovered    ReviewReason = "ZONE_NOT_COVERED"
	ReviewInvalidWindow     ReviewReason = "INVALID_WINDOW"
	ReviewCapacityExceeded  ReviewReason = "CAPACITY_EXCEEDED"
	ReviewCustomerBlocked   ReviewReason = "CUSTOMER_BLOCKED"
	ReviewAddressNotLocated ReviewReason = "ADDRESS_NOT_LOCATED"
)

// ParseReviewReason accepts only the closed set of reasons.
func ParseReviewReason(value string) (ReviewReason, error) {
	reason := ReviewReason(value)
	switch reason {
	case ReviewZoneNotCovered, ReviewInvalidWindow, ReviewCapacityExceeded, ReviewCustomerBlocked, ReviewAddressNotLocated:
		return reason, nil
	default:
		return "", ErrInvalidReviewReason
	}
}

// Spatial reports whether the reason is about where the order goes. A spatial
// problem has no operator override: without a located address inside a zone the
// order cannot be routed, so the operator must correct the address instead.
func (r ReviewReason) Spatial() bool {
	return r == ReviewZoneNotCovered || r == ReviewAddressNotLocated
}

// ApplyValidation records the outcome of the automatic validation on a newly
// registered or corrected order. No reasons means the order is PENDING and
// plannable; any reason moves it to PENDING_REVIEW, where an operator decides.
func (o Order) ApplyValidation(reasons []ReviewReason) (Order, error) {
	if o.Status != StatusPending {
		return Order{}, ErrInvalidTransition
	}
	for _, reason := range reasons {
		if _, err := ParseReviewReason(string(reason)); err != nil {
			return Order{}, err
		}
	}
	if len(reasons) == 0 {
		o.ReviewReasons = nil
		return o, nil
	}
	o.Status = StatusPendingReview
	o.ReviewReasons = append([]ReviewReason(nil), reasons...)
	return o, nil
}

// Confirm accepts an order in review as it is. Spatial reasons block it: the
// operator must correct the address or reject the order.
func (o Order) Confirm() (Order, error) {
	if o.Status != StatusPendingReview {
		return Order{}, ErrOrderNotInReview
	}
	for _, reason := range o.ReviewReasons {
		if reason.Spatial() {
			return Order{}, ErrSpatialReasonNotOverridable
		}
	}
	o.Status = StatusPending
	o.ReviewReasons = nil
	return o, nil
}

// Reject closes an order in review. The audit reason is stored by the caller.
func (o Order) Reject() (Order, error) {
	if o.Status != StatusPendingReview {
		return Order{}, ErrOrderNotInReview
	}
	o.Status = StatusCancelled
	o.ReviewReasons = nil
	return o, nil
}
