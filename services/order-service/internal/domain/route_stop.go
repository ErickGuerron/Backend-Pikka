package domain

import "errors"

// Driver errors. The messages are safe to return to the caller.
var (
	// ErrNotDriver: only the DRIVER role advances stops along its route.
	ErrNotDriver = errors.New("only drivers can advance route stops")
	// ErrInvalidAdvanceTarget: the driver can only move a stop to PICKED_UP or IN_TRANSIT here.
	// DELIVERED and NOT_DELIVERED are recorded by RegisterDelivery (driver-service).
	ErrInvalidAdvanceTarget = errors.New("route stop can only advance to PICKED_UP or IN_TRANSIT")
)

// IsRouteStopTarget reports whether status is a step the driver can take on a stop.
func IsRouteStopTarget(status Status) bool {
	return status == StatusPickedUp || status == StatusInTransit
}

// AdvanceRouteStop moves the order one step along its route: ROUTE_ASSIGNED -> PICKED_UP -> IN_TRANSIT.
// The transition table is the one in Status.CanTransitionTo. A target outside the driver steps is
// ErrInvalidAdvanceTarget; a step the table does not allow from the current status is ErrInvalidTransition.
func (o Order) AdvanceRouteStop(target Status) (Order, error) {
	if !IsRouteStopTarget(target) {
		return o, ErrInvalidAdvanceTarget
	}
	next, err := o.TransitionTo(target)
	if err != nil {
		// TransitionTo returns a zero Order on error; a rejected step must leave the order intact.
		return o, err
	}
	return next, nil
}
