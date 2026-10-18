// Package ports declares what the routing application needs from other services.
// Adapters (gRPC clients to Order and Zone) implement these interfaces.
package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// OrderGateway reads pending orders and assigns them to routes in Order Service.
type OrderGateway interface {
	// PendingByDate returns the PENDING orders of a delivery date (YYYY-MM-DD).
	PendingByDate(ctx context.Context, deliveryDate string) ([]domain.PlannableOrder, error)

	// AssignToRoute moves the orders of the stops to ROUTE_ASSIGNED for routeID, all or none.
	// Each stop carries its planned sequence within the route.
	AssignToRoute(ctx context.Context, routeID string, stops []domain.RouteStop) error
}

// ZoneCapacities reads the per-route capacity of a zone from Zone Service.
type ZoneCapacities interface {
	// MaxOrdersPerRoute returns the zone's max_orders_per_route.
	MaxOrdersPerRoute(ctx context.Context, zoneID string) (int, error)
}

// DriverPool finds available drivers and reserves one for a route in Driver Service.
type DriverPool interface {
	// FindAvailable returns up to n IDs of drivers in status AVAILABLE, in the order Driver Service returns them.
	FindAvailable(ctx context.Context, n int) ([]string, error)

	// Reserve binds driverID to routeID and returns the driver's full name from Driver Service.
	// Driver Service rejects a driver that is already reserved for another route.
	Reserve(ctx context.Context, driverID, routeID string) (driverName string, err error)

	// Release frees the reservation of routeID held by driverID. Driver Service treats a
	// repeated release as success. Used to undo a reservation whose route could not be assigned.
	Release(ctx context.Context, driverID, routeID string) error
}
