package plan

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

var (
	// ErrBlankID is returned when the route id or the order id is blank.
	ErrBlankID = errors.New("route id and order id are required")
	// ErrOrderNotPending is returned when the order does not exist or is no longer PENDING.
	ErrOrderNotPending = errors.New("order does not exist or is not pending")
	// ErrRouteNotFound is returned when the route has no orders on the order's delivery date.
	ErrRouteNotFound = errors.New("route has no orders on the delivery date of the order")
	// ErrZoneMismatch is returned when the order belongs to another zone than the route.
	ErrZoneMismatch = errors.New("order belongs to another zone than the route")
	// ErrRouteFinished is returned when every stop of the route is already completed.
	ErrRouteFinished = errors.New("route has no open stops left")
	// ErrRouteFull is returned when the route already holds its zone's maximum orders.
	ErrRouteFull = errors.New("route is at its maximum capacity")
)

// AddToRoute puts a PENDING order at the end of a route that is still running. The route keeps its
// driver and its other stops; the new stop takes the next sequence.
type AddToRoute struct {
	orders ports.OrderGateway
	zones  ports.ZoneCapacities
}

// NewAddToRoute injects the order gateway and the zone capacities.
func NewAddToRoute(orders ports.OrderGateway, zones ports.ZoneCapacities) *AddToRoute {
	return &AddToRoute{orders: orders, zones: zones}
}

// Run validates the order and the route and assigns the order as the route's last stop.
func (uc *AddToRoute) Run(ctx context.Context, routeID, orderID string) (domain.RouteStop, error) {
	routeID, orderID = strings.TrimSpace(routeID), strings.TrimSpace(orderID)
	if routeID == "" || orderID == "" {
		return domain.RouteStop{}, ErrBlankID
	}
	order, found, err := uc.orders.FindPending(ctx, orderID)
	if err != nil {
		return domain.RouteStop{}, fmt.Errorf("load order %s: %w", orderID, err)
	}
	if !found {
		return domain.RouteStop{}, ErrOrderNotPending
	}
	onRoute, err := uc.orders.RouteOrders(ctx, order.DeliveryDate, routeID)
	if err != nil {
		return domain.RouteStop{}, fmt.Errorf("load route %s: %w", routeID, err)
	}
	if len(onRoute) == 0 {
		return domain.RouteStop{}, ErrRouteNotFound
	}

	last, open := 0, false
	for _, stop := range onRoute {
		if stop.ZoneID != order.ZoneID {
			return domain.RouteStop{}, ErrZoneMismatch
		}
		last = max(last, stop.Sequence)
		open = open || stop.Open
	}
	if !open {
		return domain.RouteStop{}, ErrRouteFinished
	}
	capacity, err := uc.zones.MaxOrdersPerRoute(ctx, order.ZoneID)
	if err != nil {
		return domain.RouteStop{}, fmt.Errorf("zone %s capacity: %w", order.ZoneID, err)
	}
	if len(onRoute) >= capacity {
		return domain.RouteStop{}, ErrRouteFull
	}

	stop := domain.RouteStop{OrderID: order.OrderID, ZoneID: order.ZoneID, Address: order.Address, Sequence: last + 1}
	if err := uc.orders.AssignToRoute(ctx, routeID, []domain.RouteStop{stop}); err != nil {
		return domain.RouteStop{}, fmt.Errorf("assign order %s to route %s: %w", orderID, routeID, err)
	}
	return stop, nil
}
