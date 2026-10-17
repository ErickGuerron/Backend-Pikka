package domain

import (
	"errors"
	"strings"
)

// Route assignment errors. The messages are safe to return to the caller.
var (
	ErrInvalidRouteID   = errors.New("route id is required")
	ErrNoOrdersToAssign = errors.New("at least one order id is required")
	ErrDuplicateOrderID = errors.New("order ids must be unique")
	ErrInvalidOrderID   = errors.New("order id must be a UUID")
	ErrOrderNotPending  = errors.New("only PENDING orders can be assigned to a route")
	// ErrInvalidStopSequence: a stop sequence is a position, so it cannot be negative.
	// Callers that send stops require 1 or more; 0 is reserved for "unassigned".
	ErrInvalidStopSequence   = errors.New("stop sequence must be at least 1")
	ErrDuplicateStopSequence = errors.New("stop sequences must be unique within a route")
)

// AssignToRoute moves a PENDING order to ROUTE_ASSIGNED and records the route and its stop sequence.
// stopSequence 0 leaves the order unassigned in sequence. Any other state, a blank route, or a
// negative sequence is rejected and the order is returned unchanged.
func (o Order) AssignToRoute(routeID string, stopSequence int) (Order, error) {
	if strings.TrimSpace(routeID) == "" {
		return o, ErrInvalidRouteID
	}
	if stopSequence < 0 {
		return o, ErrInvalidStopSequence
	}
	if o.Status != StatusPending {
		return o, ErrOrderNotPending
	}
	next, err := o.TransitionTo(StatusRouteAssigned)
	if err != nil {
		return o, err
	}
	next.RouteID = routeID
	next.StopSequence = stopSequence
	return next, nil
}
