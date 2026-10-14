package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// OrderRepository es el contrato de persistencia de Order.
// La implementación vive en internal/adapters/persistence.
type OrderRepository interface {
	// Create persiste un pedido ya validado. Si notice no es nil, la notificación
	// se guarda en la misma transacción.
	Create(ctx context.Context, order domain.Order, notice *domain.Notification) (domain.Order, error)

	// ListByCustomer devuelve los pedidos del cliente, del más reciente al más antiguo.
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]domain.Order, error)

	// ListExceptions devuelve la cola de excepciones: pedidos en PENDING_REVIEW, del más antiguo al más reciente.
	ListExceptions(ctx context.Context) ([]domain.Order, error)

	// Review aplica una decisión del operador dentro de una transacción con bloqueo de fila.
	// decide recibe el pedido actual y devuelve el pedido nuevo y la notificación (opcional).
	// Devuelve domain.ErrOrderNotFound si el id no existe.
	Review(ctx context.Context, orderID string, decide func(current domain.Order) (domain.Order, *domain.Notification, error)) (domain.Order, error)
}

// NotificationRepository lee los avisos del cliente.
type NotificationRepository interface {
	// ListByCustomer devuelve los avisos del cliente, del más reciente al más antiguo.
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]domain.Notification, error)
}

// ZoneClassifier resuelve la zona que cubre un punto consultando Zone Service.
// Devuelve el id de la zona, o domain.ErrZoneNotCovered si ninguna la contiene.
type ZoneClassifier interface {
	ClassifyPoint(ctx context.Context, latitude, longitude float64) (zoneID string, err error)
}

// Geocoder convierte una dirección en coordenadas. found es false si el proveedor
// no encontró la dirección; err solo indica que el proveedor no respondió.
type Geocoder interface {
	Locate(ctx context.Context, address string) (latitude, longitude float64, found bool, err error)
}
