// Package plan generates route plans for one delivery date and assigns them to Order Service.
package plan

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// ErrInvalidDate is returned when the delivery date is not a YYYY-MM-DD calendar date.
var ErrInvalidDate = errors.New("delivery date must be a valid YYYY-MM-DD date")

const dateLayout = "2006-01-02"

// Result lists the routes assigned in Order Service. On error it holds only the
// routes that were assigned before the failure.
type Result struct {
	Assigned []domain.Route
}

// Generate plans the PENDING orders of a date and assigns each route to Order Service.
//
// Planning finishes for every zone before the first assignment, so a planning error
// changes no order. Each route is assigned in its own all-or-none call; routes are not
// atomic together, so a failure is reported with the routes already assigned.
type Generate struct {
	orders  ports.OrderGateway
	zones   ports.ZoneCapacities
	planner domain.RoutePlanner
}

// New injects the gateways and the planner.
func New(orders ports.OrderGateway, zones ports.ZoneCapacities, planner domain.RoutePlanner) *Generate {
	return &Generate{orders: orders, zones: zones, planner: planner}
}

// Run generates and assigns the routes for deliveryDate (YYYY-MM-DD).
func (uc *Generate) Run(ctx context.Context, deliveryDate string) (Result, error) {
	if _, err := time.Parse(dateLayout, deliveryDate); err != nil {
		return Result{}, ErrInvalidDate
	}
	pending, err := uc.orders.PendingByDate(ctx, deliveryDate)
	if err != nil {
		return Result{}, fmt.Errorf("load pending orders: %w", err)
	}

	planned, err := uc.planByZone(ctx, pending)
	if err != nil {
		return Result{}, err
	}

	for i, route := range planned {
		if err := uc.orders.AssignToRoute(ctx, scopedRouteID(deliveryDate, route.ID), orderIDs(route)); err != nil {
			return Result{Assigned: planned[:i]}, fmt.Errorf("assign route %s: %w", route.ID, err)
		}
	}
	return Result{Assigned: planned}, nil
}

// planByZone groups the orders by zone, in zone order, and plans each zone with its own capacity.
func (uc *Generate) planByZone(ctx context.Context, pending []domain.PlannableOrder) ([]domain.Route, error) {
	groups := make(map[string][]domain.PlannableOrder)
	for _, order := range pending {
		groups[order.ZoneID] = append(groups[order.ZoneID], order)
	}

	var planned []domain.Route
	for _, zoneID := range slices.Sorted(maps.Keys(groups)) {
		if zoneID == "" {
			// Never ask Zone Service about a blank zone: the order itself is invalid.
			return nil, fmt.Errorf("plan: %w", domain.ErrInvalidOrder)
		}
		capacity, err := uc.zones.MaxOrdersPerRoute(ctx, zoneID)
		if err != nil {
			return nil, fmt.Errorf("zone %s capacity: %w", zoneID, err)
		}
		routes, err := uc.planner.Plan(groups[zoneID], capacity)
		if err != nil {
			return nil, fmt.Errorf("plan zone %s: %w", zoneID, err)
		}
		planned = append(planned, routes...)
	}
	return planned, nil
}

// scopedRouteID prefixes the planner id with the delivery date. Planner ids are
// {zone}#{n} and repeat on every date, so the date keeps them unique in Order Service.
func scopedRouteID(deliveryDate, plannerID string) string {
	return deliveryDate + "/" + plannerID
}

func orderIDs(route domain.Route) []string {
	ids := make([]string, 0, len(route.Stops))
	for _, stop := range route.Stops {
		ids = append(ids, stop.OrderID)
	}
	return ids
}
