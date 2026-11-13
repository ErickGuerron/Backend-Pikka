package order

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// deliveryDateLayout is the calendar date format of delivery_date (YYYY-MM-DD).
const deliveryDateLayout = "2006-01-02"

// RouteStop is one order of a route and its position in it (1 is the first stop).
type RouteStop struct {
	OrderID  string
	Sequence int
	Version  int64
}

// AssignToRouteCommand pide asignar pedidos PENDING a una ruta. Role es la identidad
// propagada por el Gateway: solo ADMIN y OPERATOR planifican rutas.
// Stops es la forma preferida, con la secuencia de cada parada. Si viene vacío se usa
// OrderIDs (forma heredada): los pedidos quedan sin secuencia (0).
type AssignToRouteCommand struct {
	Role     string
	RouteID  string
	OrderIDs []string
	Stops    []RouteStop
}

// RouteAssignment expone los pedidos pendientes por fecha y su asignación a ruta.
type RouteAssignment struct {
	orders         ports.RouteAssignmentRepository
	queue          *ZoneQueue
	capacityOrders ports.CapacityRouteAssignmentRepository
}

// NewRouteAssignment inyecta el repositorio.
func NewRouteAssignment(orders ports.RouteAssignmentRepository) *RouteAssignment {
	return &RouteAssignment{orders: orders}
}

// WithZoneQueue connects planning to the same zone capacity used for queue fills.
func (uc *RouteAssignment) WithZoneQueue(queue *ZoneQueue, orders ports.CapacityRouteAssignmentRepository) *RouteAssignment {
	uc.queue = queue
	uc.capacityOrders = orders
	return uc
}

// PendingByDate devuelve los pedidos PENDING de la fecha de entrega indicada.
func (uc *RouteAssignment) PendingByDate(ctx context.Context, role, deliveryDate string) ([]domain.Order, error) {
	if !isOperator(role) {
		return nil, domain.ErrNotOperator
	}
	if _, err := time.Parse(deliveryDateLayout, deliveryDate); err != nil {
		return nil, domain.ErrInvalidDate
	}
	pending, err := uc.orders.ListPendingByDate(ctx, deliveryDate)
	if err != nil || uc.queue == nil {
		return pending, err
	}
	remaining := make(map[string]int)
	eligible := make([]domain.Order, 0, len(pending))
	for _, o := range pending {
		if o.ZoneID == "" {
			continue
		}
		free, seen := remaining[o.ZoneID]
		if !seen {
			limits, count, err := uc.queue.capacityForDay(ctx, o.ZoneID, deliveryDate)
			if err != nil {
				return nil, err
			}
			if limits.PerRoute > 0 {
				free = count
			}
			remaining[o.ZoneID] = free
		}
		if free > 0 {
			eligible = append(eligible, o)
			remaining[o.ZoneID]--
		}
	}
	return eligible, nil
}

// Assign valida la entrada y mueve todos los pedidos a ROUTE_ASSIGNED, o ninguno.
func (uc *RouteAssignment) Assign(ctx context.Context, cmd AssignToRouteCommand) ([]domain.Order, error) {
	if !isOperator(cmd.Role) {
		return nil, domain.ErrNotOperator
	}
	routeID := strings.TrimSpace(cmd.RouteID)
	if routeID == "" {
		return nil, domain.ErrInvalidRouteID
	}
	stops, err := normalizeStops(cmd)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(stops))
	byID := make(map[string]RouteStop, len(stops))
	for _, stop := range stops {
		ids = append(ids, stop.OrderID)
		byID[stop.OrderID] = stop
	}
	decide := func(current domain.Order) (domain.Order, error) {
		stop := byID[current.ID]
		if stop.Version > 0 && stop.Version != current.Version {
			return domain.Order{}, domain.ErrZoneCapacityChanged
		}
		return current.AssignToRoute(routeID, stop.Sequence)
	}
	if uc.queue != nil && uc.capacityOrders != nil {
		// The sweep and other hooks can fill later. Filling here could claim orders from
		// another route in the same routing plan before Routing commits that route.
		return uc.capacityOrders.AssignWithinCapacity(ctx, ids, routeID, uc.queue.capacity, decide)
	}
	return uc.orders.AssignToRoute(ctx, ids, decide)
}

// normalizeStops validates the stops (or the legacy order ids) before any repository call.
// Stops win when both forms are sent. Sequences must be unique within the route and 1 or more.
func normalizeStops(cmd AssignToRouteCommand) ([]RouteStop, error) {
	withSequence := len(cmd.Stops) > 0
	source := cmd.Stops
	if !withSequence {
		if len(cmd.OrderIDs) == 0 {
			return nil, domain.ErrNoOrdersToAssign
		}
		source = make([]RouteStop, 0, len(cmd.OrderIDs))
		for _, raw := range cmd.OrderIDs {
			source = append(source, RouteStop{OrderID: raw})
		}
	}
	out := make([]RouteStop, 0, len(source))
	seenIDs := make(map[uuid.UUID]struct{}, len(source))
	seenSequences := make(map[int]struct{}, len(source))
	for _, stop := range source {
		id, err := uuid.Parse(stop.OrderID)
		if err != nil {
			return nil, domain.ErrInvalidOrderID
		}
		if _, dup := seenIDs[id]; dup {
			return nil, domain.ErrDuplicateOrderID
		}
		seenIDs[id] = struct{}{}
		if withSequence {
			if stop.Sequence < 1 {
				return nil, domain.ErrInvalidStopSequence
			}
			if _, dup := seenSequences[stop.Sequence]; dup {
				return nil, domain.ErrDuplicateStopSequence
			}
			seenSequences[stop.Sequence] = struct{}{}
		}
		out = append(out, RouteStop{OrderID: id.String(), Sequence: stop.Sequence, Version: stop.Version})
	}
	return out, nil
}
