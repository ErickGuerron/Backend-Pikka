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

	// AssignToRoute moves the orders to ROUTE_ASSIGNED for routeID, all or none.
	AssignToRoute(ctx context.Context, routeID string, orderIDs []string) error
}

// ZoneCapacities reads the per-route capacity of a zone from Zone Service.
type ZoneCapacities interface {
	// MaxOrdersPerRoute returns the zone's max_orders_per_route.
	MaxOrdersPerRoute(ctx context.Context, zoneID string) (int, error)
}
