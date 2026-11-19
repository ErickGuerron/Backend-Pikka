package domain

import "errors"

var (
	ErrInvalidCapacity = errors.New("max orders per route must be greater than zero")
	ErrInvalidOrder    = errors.New("order id and zone id are required")
	ErrDuplicateOrder  = errors.New("duplicate order id")
)
