package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// DeliveryRecorder guarda los registros de entrega en el esquema drivers. La restricción
// UNIQUE (route_id, stop_id) hace que una parada no pueda registrarse dos veces.
type DeliveryRecorder struct {
	pool *pgxpool.Pool
}

// NewDeliveryRecorder crea el repositorio sobre un pool ya abierto.
func NewDeliveryRecorder(pool *pgxpool.Pool) *DeliveryRecorder {
	return &DeliveryRecorder{pool: pool}
}

// Record inserta el registro y su verificación de llegada.
func (r *DeliveryRecorder) Record(ctx context.Context, routeID string, rec domain.DeliveryRecord, arrival domain.ArrivalCheck) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO deliveries (route_id, stop_id, outcome, reason, recorded_at,
		    latitude, longitude, photo_url, distance_meters, within_radius, within_window)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		routeID, rec.StopID, string(rec.Outcome), string(rec.Reason), rec.At,
		rec.Location.Latitude, rec.Location.Longitude, rec.PhotoURL,
		arrival.DistanceM, arrival.WithinRadius, arrival.WithinWindow,
	)
	if isPgError(err, pgUniqueViolation) {
		return ports.ErrDeliveryAlreadyRecorded
	}
	return err
}
