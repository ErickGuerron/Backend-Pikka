package driver

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// DeliveryInput es lo que el repartidor registra al terminar una parada.
type DeliveryInput struct {
	StopID              string
	Outcome             domain.Outcome
	Reason              domain.NotDeliveredReason
	At                  time.Time
	Location            domain.GeoPoint
	PhotoURL            string
	OutsideWindowReason string
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
// debe estar ON_ROUTE en esa ruta. stop trae la ubicación y la ventana de entrega.
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
	if saved, err := d.replay(ctx, routeID, rec); !errors.Is(err, ports.ErrDeliveryNotFound) {
		return saved, err
	}
	arrival, err := domain.VerifyArrival(stop, rec, d.radius)
	if err != nil {
		return Registration{}, err
	}
	// D1: una entrega exitosa solo se acepta dentro del radio. Un no entregado se registra siempre.
	if rec.Outcome == domain.OutcomeDelivered && !arrival.WithinRadius {
		return Registration{}, domain.ErrOutsideRadius
	}
	if rec.Outcome == domain.OutcomeDelivered && !arrival.WithinWindow {
		rec.OutsideWindowReason = strings.TrimSpace(in.OutsideWindowReason)
		if rec.OutsideWindowReason == "" {
			return Registration{}, domain.ErrOutsideWindowReasonRequired
		}
		if utf8.RuneCountInString(rec.OutsideWindowReason) > 300 || strings.ContainsRune(rec.OutsideWindowReason, 0) {
			return Registration{}, domain.ErrInvalidOutsideWindowReason
		}
	}
	if err := d.recorder.Record(ctx, routeID, rec, arrival); err != nil {
		if errors.Is(err, ports.ErrDeliveryAlreadyRecorded) {
			return d.replay(ctx, routeID, rec)
		}
		return Registration{}, err
	}
	return Registration{Record: rec, Arrival: arrival}, nil
}

// A retry may have a new timestamp and GPS fix. Keep the first accepted evidence.
func (d *Deliveries) replay(ctx context.Context, routeID string, requested domain.DeliveryRecord) (Registration, error) {
	saved, arrival, err := d.recorder.Find(ctx, routeID, requested.StopID)
	if err != nil {
		return Registration{}, err
	}
	if saved.Outcome != requested.Outcome || saved.Reason != requested.Reason {
		return Registration{}, ports.ErrDeliveryAlreadyRecorded
	}
	return Registration{Record: saved, Arrival: arrival}, nil
}
