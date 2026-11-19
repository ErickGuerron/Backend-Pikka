package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RouteStopRepository cuenta las paradas que quedan antes de un pedido del cliente.
// La implementación vive en internal/adapters/persistence.
type RouteStopRepository interface {
	// CountStopsBefore cuenta los pedidos de la misma ruta con menor secuencia que el pedido indicado y que
	// siguen abiertos (no DELIVERED, NOT_DELIVERED ni CANCELLED). Si el pedido no existe o es de otro
	// cliente devuelve domain.ErrOrderNotFound (BOLA, ADR 0007). Un pedido sin ruta o sin secuencia cuenta 0.
	CountStopsBefore(ctx context.Context, orderID string, customerID uuid.UUID) (int, error)
}

// DriverRouteStopRepository lista las paradas de una ruta para su conductor.
type DriverRouteStopRepository interface {
	// ListDriverRouteStops returns the orders assigned to a route, ordered by stop_sequence. CANCELLED
	// orders are left out because they keep their stop_sequence.
	ListDriverRouteStops(ctx context.Context, routeID string) ([]domain.Order, error)
}
