package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// DeliveryRecorder guarda los registros de entrega en el esquema drivers. La restricción
// UNIQUE (route_id, stop_id) hace que una parada no pueda registrarse dos veces.
type DeliveryRecorder struct {
	pool *pgxpool.Pool
}

func (r *DeliveryRecorder) Find(ctx context.Context, routeID, stopID string) (domain.DeliveryRecord, domain.ArrivalCheck, error) {
	rec := domain.DeliveryRecord{StopID: stopID}
	var arrival domain.ArrivalCheck
	err := r.pool.QueryRow(ctx, `
		SELECT outcome, reason, recorded_at, latitude, longitude, photo_url,
		       distance_meters, within_radius, within_window, outside_window_reason
		FROM deliveries WHERE route_id = $1 AND stop_id = $2`, routeID, stopID,
	).Scan(&rec.Outcome, &rec.Reason, &rec.At, &rec.Location.Latitude, &rec.Location.Longitude,
		&rec.PhotoURL, &arrival.DistanceM, &arrival.WithinRadius, &arrival.WithinWindow, &rec.OutsideWindowReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, arrival, ports.ErrDeliveryNotFound
	}
	return rec, arrival, err
}

// NewDeliveryRecorder crea el repositorio sobre un pool ya abierto.
func NewDeliveryRecorder(pool *pgxpool.Pool) *DeliveryRecorder {
	return &DeliveryRecorder{pool: pool}
}

// Record inserta el registro y su verificación de llegada.
func (r *DeliveryRecorder) Record(ctx context.Context, routeID string, rec domain.DeliveryRecord, arrival domain.ArrivalCheck) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO deliveries (route_id, stop_id, outcome, reason, recorded_at,
		    latitude, longitude, photo_url, distance_meters, within_radius, within_window, outside_window_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		routeID, rec.StopID, string(rec.Outcome), string(rec.Reason), rec.At,
		rec.Location.Latitude, rec.Location.Longitude, rec.PhotoURL,
		arrival.DistanceM, arrival.WithinRadius, arrival.WithinWindow, rec.OutsideWindowReason,
	)
	if isPgError(err, pgUniqueViolation) {
		return ports.ErrDeliveryAlreadyRecorded
	}
	return err
}
