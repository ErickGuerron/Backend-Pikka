// Package persistence implementa ports.ZoneRepository sobre PostgreSQL + PostGIS.
package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// sqlUniqueViolation es el código SQLSTATE de una violación de unicidad.
const sqlUniqueViolation = "23505"

// sridWGS84 es el sistema de referencia de coordenadas lat/lng (EPSG:4326).
const sridWGS84 = 4326

// ringExterior es el índice de anillo de PostGIS para el borde exterior. Los huecos van desde 2.
const ringExterior = 1

// ZoneRepository guarda las zonas en el esquema zones. El área se almacena como
// geography(POLYGON, 4326) con índice GiST, lo que permite ST_Covers eficiente.
type ZoneRepository struct {
	pool *pgxpool.Pool
}

var _ ports.ZoneRepository = (*ZoneRepository)(nil)

// NewZoneRepository construye el repositorio sobre un pool de conexiones.
func NewZoneRepository(pool *pgxpool.Pool) *ZoneRepository {
	return &ZoneRepository{pool: pool}
}

// Save inserta la zona, su configuración y su entrada de bitácora en una misma transacción.
func (r *ZoneRepository) Save(ctx context.Context, zone domain.Zone) (domain.Zone, error) {
	actor := domain.ActorFrom(ctx)
	s := zone.Settings
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var id string
		var createdAt, updatedAt time.Time
		var version int32
		err := tx.QueryRow(ctx,
			`INSERT INTO zones (code, name, area, description, depot_name, depot_lat, depot_lng,
			                    max_orders_per_route, route_capacity_kg, timezone, service_days,
			                    delivery_window_from, delivery_window_to, cutoff_time, dispatch_time,
			                    priority, daily_order_limit, created_by, updated_by)
			 VALUES ($1, $2, ST_GeogFromText($3), $4, $5, $6, $7, $8, $9, $10, $11,
			         $12, $13, $14, $15, $16, $17, $18, $18)
			 RETURNING id::text, created_at, updated_at, version`,
			zone.Code, zone.Name, wktPolygon(zone.Ring, zone.Holes),
			s.Description, s.DepotName, s.DepotLatitude, s.DepotLongitude,
			int32(s.MaxOrdersPerRoute), s.RouteCapacityKg, s.Timezone, s.ServiceDays,
			s.DeliveryWindowFrom, s.DeliveryWindowTo, s.CutoffTime, s.DispatchTime,
			s.Priority, int32(s.DailyOrderLimit), actor,
		).Scan(&id, &createdAt, &updatedAt, &version)
		if err != nil {
			return err
		}
		zone.ID = id
		zone.CreatedAt = createdAt
		zone.UpdatedAt = updatedAt
		zone.Version = int(version)
		zone.CreatedBy = actor
		zone.UpdatedBy = actor
		return recordEvent(ctx, tx, id, "CREATED", "")
	})
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Zone{}, domain.ErrZoneCodeTaken
		}
		return domain.Zone{}, fmt.Errorf("insert zone: %w", err)
	}
	zone.Status = domain.StatusActive
	return zone, nil
}

// Update reemplaza nombre, área y configuración solo si la versión sigue siendo la que se leyó.
// Incrementa la versión y registra la bitácora en la misma transacción.
func (r *ZoneRepository) Update(ctx context.Context, zone domain.Zone) (domain.Zone, error) {
	actor := domain.ActorFrom(ctx)
	s := zone.Settings
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var createdAt, updatedAt time.Time
		var version int32
		var createdBy string
		err := tx.QueryRow(ctx,
			`UPDATE zones
			    SET name = $2, area = ST_GeogFromText($3),
			        description = $4, depot_name = $5, depot_lat = $6, depot_lng = $7,
			        max_orders_per_route = $8, route_capacity_kg = $9, timezone = $10,
			        service_days = $11, delivery_window_from = $12, delivery_window_to = $13,
			        cutoff_time = $14, dispatch_time = $15, priority = $16, daily_order_limit = $17,
			        updated_by = $18,
			        version = version + 1, updated_at = now()
			  WHERE id = $1 AND version = $19
			  RETURNING created_at, updated_at, version, created_by`,
			zone.ID, zone.Name, wktPolygon(zone.Ring, zone.Holes),
			s.Description, s.DepotName, s.DepotLatitude, s.DepotLongitude,
			int32(s.MaxOrdersPerRoute), s.RouteCapacityKg, s.Timezone,
			s.ServiceDays, s.DeliveryWindowFrom, s.DeliveryWindowTo,
			s.CutoffTime, s.DispatchTime, s.Priority, int32(s.DailyOrderLimit),
			actor, int32(zone.Version),
		).Scan(&createdAt, &updatedAt, &version, &createdBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return r.versionOrNotFound(ctx, tx, zone.ID)
		}
		if err != nil {
			return err
		}
		zone.CreatedAt = createdAt
		zone.UpdatedAt = updatedAt
		zone.Version = int(version)
		zone.CreatedBy = createdBy
		zone.UpdatedBy = actor
		return recordEvent(ctx, tx, zone.ID, "UPDATED", "")
	})
	if err != nil {
		return domain.Zone{}, mapError("update zone", err)
	}
	return zone, nil
}

// versionOrNotFound distingue "la zona no existe" de "la versión cambió mientras editabas".
func (r *ZoneRepository) versionOrNotFound(ctx context.Context, tx pgx.Tx, id string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM zones WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.ErrZoneNotFound
	}
	return domain.ErrVersionConflict
}

// SetStatus cambia el estado y guarda el motivo. La bitácora distingue desactivar de reactivar.
func (r *ZoneRepository) SetStatus(ctx context.Context, id, status, reason string) (domain.Zone, error) {
	action := "REACTIVATED"
	if status == domain.StatusInactive {
		action = "DEACTIVATED"
	}
	actor := domain.ActorFrom(ctx)
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE zones
			    SET status = $2, status_reason = $3, updated_by = $4,
			        version = version + 1, updated_at = now()
			  WHERE id = $1`,
			id, status, reason, actor,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrZoneNotFound
		}
		return recordEvent(ctx, tx, id, action, reason)
	})
	if err != nil {
		return domain.Zone{}, mapError("set zone status", err)
	}
	return r.FindByID(ctx, id)
}

// Delete borra una zona inactiva y deja en la bitácora el motivo del borrado. Bloquea la fila
// mientras comprueba el estado para que nadie la reactive a mitad del borrado.
func (r *ZoneRepository) Delete(ctx context.Context, id, reason string) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM zones WHERE id = $1 FOR UPDATE`, id).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrZoneNotFound
		}
		if err != nil {
			return err
		}
		if status != domain.StatusInactive {
			return domain.ErrZoneNotInactive
		}
		if _, err := tx.Exec(ctx, `DELETE FROM zones WHERE id = $1`, id); err != nil {
			return err
		}
		return recordEvent(ctx, tx, id, "DELETED", reason)
	})
}

// NextCode reserva el siguiente número del prefijo. Una sola sentencia lo incrementa de forma
// atómica, así que dos altas simultáneas nunca reciben el mismo número.
func (r *ZoneRepository) NextCode(ctx context.Context, prefix string) (int, error) {
	var next int
	err := r.pool.QueryRow(ctx,
		`INSERT INTO zone_code_sequences (prefix, last_value) VALUES ($1, 1)
		 ON CONFLICT (prefix) DO UPDATE SET last_value = zone_code_sequences.last_value + 1
		 RETURNING last_value`,
		prefix,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next code: %w", err)
	}
	return next, nil
}

// FindByID carga la zona con su estado, su configuración, su anillo exterior y sus huecos.
func (r *ZoneRepository) FindByID(ctx context.Context, id string) (domain.Zone, error) {
	rows, err := r.pool.Query(ctx, ringPointsQuery+` WHERE z.id = $1 ORDER BY d.path`, id)
	if err != nil {
		return domain.Zone{}, fmt.Errorf("select zone: %w", err)
	}
	defer rows.Close()

	zones, err := collectZones(rows)
	if err != nil {
		return domain.Zone{}, err
	}
	if len(zones) == 0 {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	return zones[0], nil
}

// ListAll devuelve todas las zonas con sus anillos, ordenadas por código, en una sola consulta.
func (r *ZoneRepository) ListAll(ctx context.Context) ([]domain.Zone, error) {
	rows, err := r.pool.Query(ctx, ringPointsQuery+` ORDER BY z.code, z.id, d.path`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	return collectZones(rows)
}

// FindContaining devuelve la zona activa que cubre el punto. Si hay solapamiento elige la de menor
// área (y, a igualdad, la de id menor). ST_Covers excluye los puntos dentro de un hueco.
func (r *ZoneRepository) FindContaining(ctx context.Context, point domain.Coordinate) (domain.Zone, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id::text
		   FROM zones
		  WHERE status = 'ACTIVE'
		    AND ST_Covers(area, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography)
		  ORDER BY ST_Area(area) ASC, id ASC
		  LIMIT 1`,
		point.Longitude, point.Latitude,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	if err != nil {
		return domain.Zone{}, fmt.Errorf("classify point: %w", err)
	}
	return r.FindByID(ctx, id)
}

// inTx ejecuta fn dentro de una transacción: si fn devuelve error, nada se guarda.
func (r *ZoneRepository) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no hace nada si ya se confirmó

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// recordEvent registra un cambio en la bitácora. Sin usuario identificado no se registra nada:
// un cambio sin autor no es auditable, así que el error detiene la operación.
func recordEvent(ctx context.Context, tx pgx.Tx, zoneID, action, reason string) error {
	actor := domain.ActorFrom(ctx)
	if actor == "" {
		return errors.New("audit: missing actor in context")
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO zone_events (zone_id, action, actor_id, reason) VALUES ($1, $2, $3, $4)`,
		zoneID, action, actor, reason,
	)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}
	return nil
}

// mapError traduce los errores de PostgreSQL a errores de dominio cuando corresponde.
func mapError(what string, err error) error {
	switch {
	case errors.Is(err, domain.ErrZoneNotFound),
		errors.Is(err, domain.ErrZoneNotInactive),
		errors.Is(err, domain.ErrVersionConflict):
		return err
	case isUniqueViolation(err):
		return domain.ErrZoneCodeTaken
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

// ringPointsQuery devuelve los puntos de todos los anillos de cada zona, con sus datos.
// En ST_DumpPoints, path[1] es el índice del anillo: 1 es el exterior y 2 en adelante, los huecos.
const ringPointsQuery = `SELECT z.id::text, z.code, z.name, z.status, z.status_reason,
		        z.description, z.depot_name, z.depot_lat, z.depot_lng,
		        z.max_orders_per_route, z.route_capacity_kg, z.timezone, z.service_days,
		        z.delivery_window_from, z.delivery_window_to, z.cutoff_time, z.dispatch_time,
		        z.priority, z.daily_order_limit,
		        z.version, z.created_at, z.updated_at, z.created_by, z.updated_by,
		        d.path[1], ST_Y(d.geom), ST_X(d.geom)
		   FROM zones z
		   CROSS JOIN LATERAL ST_DumpPoints(z.area::geometry) AS d`

// collectZones agrupa las filas de ringPointsQuery por zona, manteniendo el orden de llegada.
// Las filas de una misma zona deben venir juntas, lo que garantizan las consultas que la usan.
func collectZones(rows pgx.Rows) ([]domain.Zone, error) {
	var zones []domain.Zone
	index := map[string]int{}
	for rows.Next() {
		var (
			id, code, name, status, reason string
			description, depotName         string
			depotLat, depotLng             *float64 // NULL en zonas anteriores a la migración 000003
			maxOrders, dailyLimit, version int32
			routeCapacity                  float64
			timezone, from, to             string
			cutoff, dispatch, priority     string
			serviceDays                    []string
			createdAt, updatedAt           time.Time
			createdBy, updatedBy           string
			ringIndex                      int32
			lat, lng                       float64
		)
		if err := rows.Scan(&id, &code, &name, &status, &reason,
			&description, &depotName, &depotLat, &depotLng,
			&maxOrders, &routeCapacity, &timezone, &serviceDays,
			&from, &to, &cutoff, &dispatch,
			&priority, &dailyLimit,
			&version, &createdAt, &updatedAt, &createdBy, &updatedBy,
			&ringIndex, &lat, &lng); err != nil {
			return nil, fmt.Errorf("scan ring point: %w", err)
		}
		i, seen := index[id]
		if !seen {
			zones = append(zones, domain.Zone{
				ID: id, Code: code, Name: name, Status: status, StatusReason: reason,
				Settings: domain.Settings{
					Description:        description,
					DepotName:          depotName,
					DepotLatitude:      derefFloat(depotLat),
					DepotLongitude:     derefFloat(depotLng),
					MaxOrdersPerRoute:  int(maxOrders),
					RouteCapacityKg:    routeCapacity,
					Timezone:           timezone,
					ServiceDays:        serviceDays,
					DeliveryWindowFrom: from,
					DeliveryWindowTo:   to,
					CutoffTime:         cutoff,
					DispatchTime:       dispatch,
					Priority:           priority,
					DailyOrderLimit:    int(dailyLimit),
				},
				Version:   int(version),
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				CreatedBy: createdBy,
				UpdatedBy: updatedBy,
			})
			i = len(zones) - 1
			index[id] = i
		}
		attachRingPoint(&zones[i], int(ringIndex), domain.Coordinate{Latitude: lat, Longitude: lng})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read rings: %w", err)
	}
	return zones, nil
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// attachRingPoint coloca un punto en el anillo exterior o en el hueco que indica el índice de PostGIS.
func attachRingPoint(zone *domain.Zone, ringIndex int, p domain.Coordinate) {
	if ringIndex == ringExterior {
		zone.Ring = append(zone.Ring, p)
		return
	}
	holeIndex := ringIndex - ringExterior - 1
	for len(zone.Holes) <= holeIndex {
		zone.Holes = append(zone.Holes, nil)
	}
	zone.Holes[holeIndex] = append(zone.Holes[holeIndex], p)
}

// wktPolygon construye un POLYGON WKT con SRID: primero el exterior y luego los huecos.
// WKT usa el orden longitud latitud.
func wktPolygon(ring []domain.Coordinate, holes [][]domain.Coordinate) string {
	rings := make([]string, 0, 1+len(holes))
	rings = append(rings, wktRing(ring))
	for _, hole := range holes {
		rings = append(rings, wktRing(hole))
	}
	return "SRID=" + strconv.Itoa(sridWGS84) + ";POLYGON(" + strings.Join(rings, ", ") + ")"
}

func wktRing(ring []domain.Coordinate) string {
	pts := make([]string, 0, len(ring))
	for _, p := range ring {
		pts = append(pts, formatFloat(p.Longitude)+" "+formatFloat(p.Latitude))
	}
	return "(" + strings.Join(pts, ", ") + ")"
}

// formatFloat evita la notación exponencial, que PostGIS acepta pero es menos legible en logs.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == sqlUniqueViolation
}
