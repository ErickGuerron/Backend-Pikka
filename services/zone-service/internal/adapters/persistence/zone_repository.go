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

// Save inserta la zona. Usa parámetros para todos los valores de usuario.
func (r *ZoneRepository) Save(ctx context.Context, zone domain.Zone) (domain.Zone, error) {
	var id string
	var createdAt time.Time
	err := r.pool.QueryRow(ctx,
		`INSERT INTO zones (code, name, area)
		 VALUES ($1, $2, ST_GeogFromText($3))
		 RETURNING id::text, created_at`,
		zone.Code, zone.Name, wktPolygon(zone.Ring),
	).Scan(&id, &createdAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Zone{}, domain.ErrZoneCodeTaken
		}
		return domain.Zone{}, fmt.Errorf("insert zone: %w", err)
	}

	zone.ID = id
	zone.CreatedAt = createdAt
	return zone, nil
}

// FindByID carga la zona y su anillo.
func (r *ZoneRepository) FindByID(ctx context.Context, id string) (domain.Zone, error) {
	var zone domain.Zone
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, code, name, created_at FROM zones WHERE id = $1`, id,
	).Scan(&zone.ID, &zone.Code, &zone.Name, &zone.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	if err != nil {
		return domain.Zone{}, fmt.Errorf("select zone: %w", err)
	}

	ring, err := r.loadRing(ctx, id)
	if err != nil {
		return domain.Zone{}, err
	}
	zone.Ring = ring
	return zone, nil
}

// FindContaining devuelve la zona que cubre el punto. Si hay solapamiento
// elige la de menor área (y, a igualdad, la de id menor) para que el resultado
// sea determinista.
func (r *ZoneRepository) FindContaining(ctx context.Context, point domain.Coordinate) (domain.Zone, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id::text
		   FROM zones
		  WHERE ST_Covers(area, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography)
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

// loadRing lee los vértices del polígono en el orden en que se guardaron.
func (r *ZoneRepository) loadRing(ctx context.Context, id string) ([]domain.Coordinate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ST_Y(d.geom), ST_X(d.geom)
		   FROM zones z
		   CROSS JOIN LATERAL ST_DumpPoints(z.area::geometry) AS d
		  WHERE z.id = $1
		  ORDER BY d.path`, id)
	if err != nil {
		return nil, fmt.Errorf("select ring: %w", err)
	}
	defer rows.Close()

	var ring []domain.Coordinate
	for rows.Next() {
		var lat, lng float64
		if err := rows.Scan(&lat, &lng); err != nil {
			return nil, fmt.Errorf("scan ring point: %w", err)
		}
		ring = append(ring, domain.Coordinate{Latitude: lat, Longitude: lng})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ring: %w", err)
	}
	return ring, nil
}

// wktPolygon construye un POLYGON WKT con SRID. WKT usa el orden longitud latitud.
func wktPolygon(ring []domain.Coordinate) string {
	pts := make([]string, 0, len(ring))
	for _, p := range ring {
		pts = append(pts, formatFloat(p.Longitude)+" "+formatFloat(p.Latitude))
	}
	return "SRID=" + strconv.Itoa(sridWGS84) + ";POLYGON((" + strings.Join(pts, ", ") + "))"
}

// formatFloat evita la notación exponencial, que PostGIS acepta pero es menos legible en logs.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == sqlUniqueViolation
}
