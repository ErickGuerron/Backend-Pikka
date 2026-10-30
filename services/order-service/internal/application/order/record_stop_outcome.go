package order

import (
	"context"
	"log/slog"
	"strings"
	"time"

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
	// Reason is the NOT_DELIVERED reason Driver Service already validated. It decides the surcharge.
	Reason string
}

// RecordStopOutcome mueve un pedido IN_TRANSIT a DELIVERED o NOT_DELIVERED dentro de su ruta.
// Reutiliza la misma transacción con bloqueo que AdvanceRouteStop, para que el alcance (pedido
// dentro de la ruta) y el estado se lean y escriban bajo el mismo bloqueo.
type RecordStopOutcome struct {
	orders     ports.OrderRepository
	queue      *ZoneQueue
	surcharges ports.SurchargeRepository
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

// WithSurcharges records a penalty incident when a stop closes NOT_DELIVERED for a customer reason,
// or a PENDING_VERIFICATION one for a wrong address.
func (uc *RecordStopOutcome) WithSurcharges(surcharges ports.SurchargeRepository) *RecordStopOutcome {
	uc.surcharges = surcharges
	return uc
}

// recordSurchargeNonFatal runs after the stop is already closed. A failure is logged and not
// returned: the driver cannot retry a closed stop, and the unique order_id keeps a later retry safe.
func (uc *RecordStopOutcome) recordSurchargeNonFatal(ctx context.Context, closed domain.Order, reason string) {
	if uc.surcharges == nil || closed.Status != domain.StatusNotDelivered || (!domain.NotDeliveredReason(reason).ChargesSurcharge() && !domain.NotDeliveredReason(reason).NeedsVerification()) {
		return
	}
	incident, err := domain.NewSurchargeIncident(closed.CustomerID, closed.ID, reason, time.Now())
	if err == nil {
		_, _, err = uc.surcharges.RecordSurchargeIncident(ctx, incident)
	}
	if err != nil {
		slog.Default().Error("surcharge incident not recorded; it must be recorded by hand", "order", closed.ID, "reason", reason, "error", err)
	}
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
	uc.recordSurchargeNonFatal(ctx, closed, cmd.Reason)
	// The closed stop frees a slot of the route's zone: the oldest waiting order can take it.
	uc.queue.fillNonFatal(ctx, closed.ZoneID, closed.DeliveryDate.String())
	return closed, nil
}
