package domain

import (
	"cmp"
	"fmt"
	"slices"
)

// RoutePlanner agrupa pedidos en rutas de capacidad acotada.
type RoutePlanner interface {
	Plan(orders []PlannableOrder, maxPerRoute int) ([]Route, error)
}

// ZoneCapacityPlanner agrupa por zona y parte cada zona en rutas de a lo sumo
// maxPerRoute paradas. No tiene estado: la capacidad llega siempre por parametro.
type ZoneCapacityPlanner struct{}

var _ RoutePlanner = ZoneCapacityPlanner{}

// Plan es determinista: el resultado depende solo del conjunto de pedidos y de
// maxPerRoute, nunca del orden de entrada. No modifica orders. Ante un error
// devuelve nil y el error.
func (ZoneCapacityPlanner) Plan(orders []PlannableOrder, maxPerRoute int) ([]Route, error) {
	if maxPerRoute <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidCapacity, maxPerRoute)
	}
	if err := validateOrders(orders); err != nil {
		return nil, err
	}

	routes := []Route{}
	sorted := slices.Clone(orders)
	slices.SortFunc(sorted, func(a, b PlannableOrder) int {
		return cmp.Or(cmp.Compare(a.ZoneID, b.ZoneID), cmp.Compare(a.OrderID, b.OrderID))
	})

	chunk := 0
	for _, o := range sorted {
		last := len(routes) - 1
		if last < 0 || routes[last].ZoneID != o.ZoneID {
			chunk = 0
			last = -1
		}
		if last < 0 || len(routes[last].Stops) == maxPerRoute {
			chunk++
			routes = append(routes, Route{ID: routeID(o.ZoneID, chunk), ZoneID: o.ZoneID})
			last = len(routes) - 1
		}
		route := &routes[last]
		route.Stops = append(route.Stops, RouteStop{OrderID: o.OrderID, ZoneID: o.ZoneID, Sequence: len(route.Stops) + 1})
	}
	return routes, nil
}

func validateOrders(orders []PlannableOrder) error {
	seen := make(map[string]struct{}, len(orders))
	for i, o := range orders {
		if err := o.Validate(); err != nil {
			return fmt.Errorf("order at index %d: %w", i, err)
		}
		if _, dup := seen[o.OrderID]; dup {
			return fmt.Errorf("%w: %q", ErrDuplicateOrder, o.OrderID)
		}
		seen[o.OrderID] = struct{}{}
	}
	return nil
}
