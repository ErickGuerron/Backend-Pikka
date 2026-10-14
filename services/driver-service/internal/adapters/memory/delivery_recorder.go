package memory

import (
	"context"
	"sync"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// Delivery es un registro guardado junto con su verificación de llegada.
type Delivery struct {
	RouteID string
	Record  domain.DeliveryRecord
	Arrival domain.ArrivalCheck
}

// DeliveryRecorder guarda registros de entrega en memoria.
type DeliveryRecorder struct {
	mu         sync.Mutex
	deliveries map[string]Delivery
}

// NewDeliveryRecorder devuelve un registro vacío.
func NewDeliveryRecorder() *DeliveryRecorder {
	return &DeliveryRecorder{deliveries: make(map[string]Delivery)}
}

// Record guarda el registro; la clave es ruta + parada.
func (r *DeliveryRecorder) Record(_ context.Context, routeID string, rec domain.DeliveryRecord, arrival domain.ArrivalCheck) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := routeID + "|" + rec.StopID
	if _, ok := r.deliveries[key]; ok {
		return ports.ErrDeliveryAlreadyRecorded
	}
	r.deliveries[key] = Delivery{RouteID: routeID, Record: rec, Arrival: arrival}
	return nil
}
