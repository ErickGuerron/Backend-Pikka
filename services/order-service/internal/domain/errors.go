package domain

import "errors"

var (
	ErrInvalidAddress    = errors.New("address must contain 5 to 300 characters")
	ErrInvalidCoordinate = errors.New("coordinates must be finite and within WGS 84 bounds")
	ErrInvalidDate       = errors.New("delivery date must be a valid YYYY-MM-DD date")
	ErrPastDeliveryDate  = errors.New("delivery date cannot precede the local registration date")
	ErrInvalidWindow     = errors.New("window must use HH:mm with end after start on the same day")
	ErrInvalidCreatedAt  = errors.New("registration instant is required")
	ErrInvalidStatus     = errors.New("unknown order status")
	ErrInvalidTransition = errors.New("order status transition is not allowed")
)
