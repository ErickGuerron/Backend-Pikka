package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RouteAssignmentRepository es el contrato de Order para la planificación de rutas
// (Routing Service). La implementación vive en internal/adapters/persistence.
type RouteAssignmentRepository interface {
	// ListPendingByDate devuelve los pedidos PENDING de una fecha de entrega
	// (YYYY-MM-DD), del más antiguo al más reciente por created_at y luego por id.
	ListPendingByDate(ctx context.Context, deliveryDate string) ([]domain.Order, error)

	// AssignToRoute aplica decide a cada pedido dentro de una transacción con bloqueo
	// de fila. Todo o nada: si un id no existe devuelve domain.ErrOrderNotFound y si
	// decide rechaza un pedido devuelve su error; en ambos casos no cambia ninguno.
	// Devuelve los pedidos en el orden recibido.
	AssignToRoute(ctx context.Context, orderIDs []string, decide func(current domain.Order) (domain.Order, error)) ([]domain.Order, error)
}
