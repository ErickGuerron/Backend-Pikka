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
)

// AssignToRoute moves a PENDING order to ROUTE_ASSIGNED and records the route.
// Any other state, or a blank route, is rejected and the order is returned unchanged.
func (o Order) AssignToRoute(routeID string) (Order, error) {
	if strings.TrimSpace(routeID) == "" {
		return o, ErrInvalidRouteID
	}
	if o.Status != StatusPending {
		return o, ErrOrderNotPending
	}
	next, err := o.TransitionTo(StatusRouteAssigned)
	if err != nil {
		return o, err
	}
	next.RouteID = routeID
	return next, nil
}
