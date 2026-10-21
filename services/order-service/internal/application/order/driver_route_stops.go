package order

import (
	"context"
	"strings"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// DriverRouteStopsCommand pide las paradas de una ruta. La ruta del conductor la verifica el Gateway
// (ADR 0008) antes de llamar; aquí solo se exige el rol.
type DriverRouteStopsCommand struct {
	Role    string
	RouteID string
}

// DriverRouteStops lista las paradas de una ruta para su conductor, en orden de secuencia.
type DriverRouteStops struct {
	stops ports.DriverRouteStopRepository
}

// NewDriverRouteStops inyecta el repositorio.
func NewDriverRouteStops(stops ports.DriverRouteStopRepository) *DriverRouteStops {
	return &DriverRouteStops{stops: stops}
}

// Run valida el rol y la ruta y devuelve los pedidos de la ruta. Un rol distinto de DRIVER no lee nada.
func (uc *DriverRouteStops) Run(ctx context.Context, cmd DriverRouteStopsCommand) ([]domain.Order, error) {
	if cmd.Role != RoleDriver {
		return nil, domain.ErrNotDriver
	}
	routeID := strings.TrimSpace(cmd.RouteID)
	if routeID == "" {
		return nil, domain.ErrInvalidRouteID
	}
	return uc.stops.ListDriverRouteStops(ctx, routeID)
}
