package order

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RecordStopOutcomeCommand cierra una parada de la ruta del conductor: DELIVERED o NOT_DELIVERED.
// La identidad y la pertenencia de la ruta las verifica el Gateway antes de llamar, y el registro
// de entrega lo acepta Driver Service antes de que este caso de uso se ejecute.
type RecordStopOutcomeCommand struct {
	Role    string
	RouteID string
	OrderID string
	Outcome domain.Status
}

// RecordStopOutcome mueve un pedido IN_TRANSIT a DELIVERED o NOT_DELIVERED dentro de su ruta.
// Reutiliza la misma transacción con bloqueo que AdvanceRouteStop, para que el alcance (pedido
// dentro de la ruta) y el estado se lean y escriban bajo el mismo bloqueo.
type RecordStopOutcome struct {
	orders ports.OrderRepository
	queue  *ZoneQueue
}

// NewRecordStopOutcome inyecta el repositorio.
func NewRecordStopOutcome(orders ports.OrderRepository) *RecordStopOutcome {
	return &RecordStopOutcome{orders: orders}
}

// WithZoneQueue hace que cerrar una parada libere su cupo para el siguiente pedido en espera de la zona.
func (uc *RecordStopOutcome) WithZoneQueue(queue *ZoneQueue) *RecordStopOutcome {
	uc.queue = queue
	return uc
}

// Run valida la entrada, aplica el cierre y devuelve el pedido actualizado.
func (uc *RecordStopOutcome) Run(ctx context.Context, cmd RecordStopOutcomeCommand) (domain.Order, error) {
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
	if !domain.IsStopOutcome(cmd.Outcome) {
		return domain.Order{}, domain.ErrInvalidStopOutcome
	}
	closed, err := uc.orders.AdvanceRouteStop(ctx, id.String(), routeID, func(current domain.Order) (domain.Order, error) {
		return current.RecordStopOutcome(cmd.Outcome)
	})
	if err != nil {
		return domain.Order{}, err
	}
	// The closed stop frees a slot of the route's zone: the oldest waiting order can take it.
	uc.queue.fillNonFatal(ctx, closed.ZoneID, closed.DeliveryDate.String())
	return closed, nil
}
