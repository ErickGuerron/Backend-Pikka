package order

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RoleDriver es el rol de la app móvil que ejecuta una ruta (ADR 0007, ADR 0008).
const RoleDriver = "DRIVER"

// AdvanceRouteStopCommand pide un paso del conductor sobre un pedido de su ruta.
// La identidad y la pertenencia de la ruta al conductor las verifica el Gateway antes de llamar.
type AdvanceRouteStopCommand struct {
	Role    string
	RouteID string
	OrderID string
	Target  domain.Status
}

// AdvanceRouteStop mueve un pedido de su ruta por ROUTE_ASSIGNED -> PICKED_UP -> IN_TRANSIT.
// El alcance (pedido dentro de la ruta) lo fija la consulta del repositorio, bajo bloqueo.
type AdvanceRouteStop struct {
	orders ports.OrderRepository
}

// NewAdvanceRouteStop inyecta el repositorio.
func NewAdvanceRouteStop(orders ports.OrderRepository) *AdvanceRouteStop {
	return &AdvanceRouteStop{orders: orders}
}

// Run valida la entrada, aplica el paso y devuelve el pedido actualizado.
func (uc *AdvanceRouteStop) Run(ctx context.Context, cmd AdvanceRouteStopCommand) (domain.Order, error) {
	if cmd.Role != RoleDriver {
		return domain.Order{}, domain.ErrNotDriver
	}
	routeID := strings.TrimSpace(cmd.RouteID)
	if routeID == "" {
		return domain.Order{}, domain.ErrInvalidRouteID
	}
	id, err := uuid.Parse(cmd.OrderID)
	if err != nil {
		return domain.Order{}, domain.ErrInvalidOrderID
	}
	if !domain.IsRouteStopTarget(cmd.Target) {
		return domain.Order{}, domain.ErrInvalidAdvanceTarget
	}
	return uc.orders.AdvanceRouteStop(ctx, id.String(), routeID, func(current domain.Order) (domain.Order, error) {
		return current.AdvanceRouteStop(cmd.Target)
	})
}
