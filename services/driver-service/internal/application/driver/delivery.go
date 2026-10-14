package driver

import (
	"context"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// DeliveryInput es lo que el repartidor registra al terminar una parada.
type DeliveryInput struct {
	StopID   string
	Outcome  domain.Outcome
	Reason   domain.NotDeliveredReason
	At       time.Time
	Location domain.GeoPoint
	PhotoURL string
}

// Registration es el resultado de registrar una entrega: el registro guardado y la
// verificación de llegada. Ninguna penalización se aplica aquí; eso lo decide el
// motor de penalizaciones a partir de Arrival.
type Registration struct {
	Record  domain.DeliveryRecord
	Arrival domain.ArrivalCheck
}

// Deliveries registra las entregas de un repartidor en ruta.
type Deliveries struct {
	drivers  ports.DriverRepository
	recorder ports.DeliveryRecorder
	radius   float64
}

// NewDeliveries crea el caso de uso. radius es el radio de llegada en metros; el
// llamador lo toma de la configuración del proceso.
func NewDeliveries(drivers ports.DriverRepository, recorder ports.DeliveryRecorder, radius float64) *Deliveries {
	return &Deliveries{drivers: drivers, recorder: recorder, radius: radius}
}

// RegisterDelivery registra el resultado de la parada stop de la ruta routeID. El repartidor
// debe estar ON_ROUTE en esa ruta. stop trae la dirección y la ventana de la parada; el
// origen de esos datos es el Routing Service, que todavía no existe.
func (d *Deliveries) RegisterDelivery(ctx context.Context, driverID, routeID string, stop domain.Stop, in DeliveryInput) (Registration, error) {
	drv, err := d.drivers.Get(ctx, driverID)
	if err != nil {
		return Registration{}, err
	}
	if drv.Status != domain.StatusOnRoute {
		return Registration{}, domain.ErrInvalidTransition
	}
	if drv.RouteID != routeID {
		return Registration{}, domain.ErrRouteMismatch
	}

	rec, err := domain.NewDeliveryRecord(in.StopID, in.Outcome, in.Reason, in.At, in.Location, in.PhotoURL)
	if err != nil {
		return Registration{}, err
	}
	arrival, err := domain.VerifyArrival(stop, rec, d.radius)
	if err != nil {
		return Registration{}, err
	}
	if err := d.recorder.Record(ctx, routeID, rec, arrival); err != nil {
		return Registration{}, err
	}
	return Registration{Record: rec, Arrival: arrival}, nil
}
