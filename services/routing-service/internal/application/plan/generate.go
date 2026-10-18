// Package plan generates route plans for one delivery date and assigns them to Order Service.
package plan

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// ErrInvalidDate is returned when the delivery date is not a YYYY-MM-DD calendar date.
var ErrInvalidDate = errors.New("delivery date must be a valid YYYY-MM-DD date")

// ErrNoDriverAvailable is returned when no AVAILABLE driver can take the next route.
var ErrNoDriverAvailable = errors.New("no driver available for route")

const (
	dateLayout = "2006-01-02"
	// releaseTimeout bounds the best-effort release of a driver after a failed assignment.
	releaseTimeout = 5 * time.Second
)

// AssignedRoute is one route that was reserved for a driver and assigned in Order Service.
// RouteID is the scoped id (delivery date plus planner id) that Order and Driver Services store.
type AssignedRoute struct {
	RouteID    string
	DriverID   string
	DriverName string
	OrderCount int
}

// Result lists the routes assigned in Order Service. On error it holds only the
// routes that were fully processed before the failure.
type Result struct {
	Assigned []domain.Route
	Routes   []AssignedRoute
}

// Generate plans the PENDING orders of a date, then reserves one driver per route and
// assigns each route to Order Service.
//
// Planning finishes for every zone before the first reservation, so a planning error
// changes nothing. Each route is processed in order: reserve its driver, then assign its
// orders. A route without a driver, or a failed reservation or assignment, stops the run
// before any order of that route is touched; routes are not atomic together, so a failure
// is reported with the routes already processed. When the assignment fails after the
// reservation, the driver is released (best effort); if the release also fails, the error
// reports both and the driver stays reserved.
type Generate struct {
	orders  ports.OrderGateway
	zones   ports.ZoneCapacities
	drivers ports.DriverPool
	planner domain.RoutePlanner
}

// New injects the gateways, the driver pool and the planner.
func New(orders ports.OrderGateway, zones ports.ZoneCapacities, drivers ports.DriverPool, planner domain.RoutePlanner) *Generate {
	return &Generate{orders: orders, zones: zones, drivers: drivers, planner: planner}
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

	var result Result
	for _, route := range planned {
		routeID := scopedRouteID(deliveryDate, route.ID)
		driverID, driverName, err := uc.reserveDriver(ctx, routeID)
		if err != nil {
			return result, err
		}
		if err := uc.orders.AssignToRoute(ctx, routeID, route.Stops); err != nil {
			assignErr := fmt.Errorf("assign route %s: %w", routeID, err)
			if releaseErr := uc.releaseDriver(ctx, driverID, routeID); releaseErr != nil {
				return result, fmt.Errorf("%w; release of driver %s also failed, the driver stays reserved: %w", assignErr, driverID, releaseErr)
			}
			return result, assignErr
		}
		result.Assigned = append(result.Assigned, route)
		result.Routes = append(result.Routes, AssignedRoute{RouteID: routeID, DriverID: driverID, DriverName: driverName, OrderCount: len(route.Stops)})
	}
	return result, nil
}

// reserveDriver takes the first AVAILABLE driver for one route. It asks for a single driver
// each time, so every route sees the drivers reserved by the routes before it.
func (uc *Generate) reserveDriver(ctx context.Context, routeID string) (driverID, driverName string, err error) {
	candidates, err := uc.drivers.FindAvailable(ctx, 1)
	if err != nil {
		return "", "", fmt.Errorf("find driver for route %s: %w", routeID, err)
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("route %s: %w", routeID, ErrNoDriverAvailable)
	}
	driverID = candidates[0]
	driverName, err = uc.drivers.Reserve(ctx, driverID, routeID)
	if err != nil {
		return "", "", fmt.Errorf("reserve driver %s for route %s: %w", driverID, routeID, err)
	}
	return driverID, driverName, nil
}

// releaseDriver undoes a reservation whose route could not be assigned. It runs even when the
// request was cancelled, because a cancelled request is a common reason for the assign to fail,
// and it is bounded by releaseTimeout so it cannot hang.
func (uc *Generate) releaseDriver(ctx context.Context, driverID, routeID string) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	return uc.drivers.Release(releaseCtx, driverID, routeID)
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
// The result is {date}-{zone}-{n}: it must stay URL-path safe, because the driver
// stop endpoint carries the route id in the path (no "/" and no "#").
func scopedRouteID(deliveryDate, plannerID string) string {
	return deliveryDate + "-" + strings.ReplaceAll(plannerID, "#", "-")
}
