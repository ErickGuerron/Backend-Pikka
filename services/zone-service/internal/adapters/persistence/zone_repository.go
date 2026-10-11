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
		zone.Code, zone.Name, wktPolygon(zone.Ring, zone.Holes),
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

	outer, holes, err := r.loadRings(ctx, id)
	if err != nil {
		return domain.Zone{}, err
	}
	zone.Ring = outer
	zone.Holes = holes
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

// loadRings lee el anillo exterior y los huecos en el orden en que se guardaron.
// En ST_DumpPoints, path[1] es el índice del anillo: 1 es el exterior y 2 en adelante, los huecos.
func (r *ZoneRepository) loadRings(ctx context.Context, id string) ([]domain.Coordinate, [][]domain.Coordinate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT d.path[1], ST_Y(d.geom), ST_X(d.geom)
		   FROM zones z
		   CROSS JOIN LATERAL ST_DumpPoints(z.area::geometry) AS d
		  WHERE z.id = $1
		  ORDER BY d.path`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("select rings: %w", err)
	}
	defer rows.Close()

	var outer []domain.Coordinate
	var holes [][]domain.Coordinate
	for rows.Next() {
		var ringIndex int32
		var lat, lng float64
		if err := rows.Scan(&ringIndex, &lat, &lng); err != nil {
			return nil, nil, fmt.Errorf("scan ring point: %w", err)
		}
		point := domain.Coordinate{Latitude: lat, Longitude: lng}
		if ringIndex == 1 {
			outer = append(outer, point)
			continue
		}
		holeIndex := int(ringIndex) - 2
		if holeIndex >= len(holes) {
			holes = append(holes, nil)
		}
		holes[holeIndex] = append(holes[holeIndex], point)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read rings: %w", err)
	}
	return outer, holes, nil
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
