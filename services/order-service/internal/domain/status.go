package domain

// Status uses the values published in OpenAPI (without the protobuf prefix).
type Status string

const (
	StatusPending       Status = "PENDING"
	StatusPendingReview Status = "PENDING_REVIEW"
	StatusRouteAssigned Status = "ROUTE_ASSIGNED"
	StatusPickedUp      Status = "PICKED_UP"
	StatusInTransit     Status = "IN_TRANSIT"
	StatusDelivered     Status = "DELIVERED"
	StatusNotDelivered  Status = "NOT_DELIVERED"
	StatusCancelled     Status = "CANCELLED"
)

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	switch status {
	case StatusPending, StatusPendingReview, StatusRouteAssigned, StatusPickedUp,
		StatusInTransit, StatusDelivered, StatusNotDelivered, StatusCancelled:
		return status, nil
	default:
		return "", ErrInvalidStatus
	}
}

// CanTransitionTo rejects skipped steps, repeated steps and changes after closure.
// PENDING_REVIEW -> CANCELLED represents rejection by an operator, not customer
// cancellation. Authorization and mandatory audit reasons belong to use cases.
func (s Status) CanTransitionTo(next Status) bool {
	switch s {
	case StatusPendingReview:
		return next == StatusPending || next == StatusCancelled
	case StatusPending:
		return next == StatusRouteAssigned || next == StatusCancelled
	case StatusRouteAssigned:
		return next == StatusPickedUp || next == StatusCancelled
	case StatusPickedUp:
		return next == StatusInTransit
	case StatusInTransit:
		return next == StatusDelivered || next == StatusNotDelivered
	default:
		return false
	}
}

func (s Status) CanCancel() bool {
	return s == StatusPending || s == StatusRouteAssigned
}
