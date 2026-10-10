package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidFullName       = fmt.Errorf("full name is required and must be at most %d characters", MaxFullNameRunes)
	ErrInvalidPhone          = fmt.Errorf("phone must have between %d and %d digits and an optional leading +", MinPhoneDigits, MaxPhoneDigits)
	ErrInvalidUserID         = errors.New("user id is required")
	ErrInvalidDriverID       = errors.New("driver id is required")
	ErrInvalidStatus         = errors.New("invalid driver status")
	ErrInvalidTransition     = errors.New("invalid status transition")
	ErrDriverInactive        = errors.New("driver is inactive")
	ErrInvalidRouteID        = errors.New("route id is required")
	ErrDriverAlreadyReserved = errors.New("driver is already reserved for another route")
	ErrDriverOffline         = errors.New("driver is offline")
	ErrRouteMismatch         = errors.New("route does not match the driver's current reservation")
)
