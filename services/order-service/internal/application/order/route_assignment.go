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

// AssignToRouteCommand pide asignar pedidos PENDING a una ruta. Role es la identidad
// propagada por el Gateway: solo ADMIN y OPERATOR planifican rutas.
type AssignToRouteCommand struct {
	Role     string
	RouteID  string
	OrderIDs []string
}

// RouteAssignment expone los pedidos pendientes por fecha y su asignación a ruta.
type RouteAssignment struct {
	orders ports.RouteAssignmentRepository
}

// NewRouteAssignment inyecta el repositorio.
func NewRouteAssignment(orders ports.RouteAssignmentRepository) *RouteAssignment {
	return &RouteAssignment{orders: orders}
}

// PendingByDate devuelve los pedidos PENDING de la fecha de entrega indicada.
func (uc *RouteAssignment) PendingByDate(ctx context.Context, role, deliveryDate string) ([]domain.Order, error) {
	if !isOperator(role) {
		return nil, domain.ErrNotOperator
	}
	if _, err := time.Parse(deliveryDateLayout, deliveryDate); err != nil {
		return nil, domain.ErrInvalidDate
	}
	return uc.orders.ListPendingByDate(ctx, deliveryDate)
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
	if len(cmd.OrderIDs) == 0 {
		return nil, domain.ErrNoOrdersToAssign
	}
	ids := make([]string, 0, len(cmd.OrderIDs))
	seen := make(map[uuid.UUID]struct{}, len(cmd.OrderIDs))
	for _, raw := range cmd.OrderIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, domain.ErrInvalidOrderID
		}
		if _, dup := seen[id]; dup {
			return nil, domain.ErrDuplicateOrderID
		}
		seen[id] = struct{}{}
		ids = append(ids, id.String())
	}
	return uc.orders.AssignToRoute(ctx, ids, func(current domain.Order) (domain.Order, error) {
		return current.AssignToRoute(routeID)
	})
}
