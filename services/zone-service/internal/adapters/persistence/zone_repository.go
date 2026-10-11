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

// Update reemplaza código, nombre y área. Incrementa la versión para dejar rastro del cambio.
func (r *ZoneRepository) Update(ctx context.Context, zone domain.Zone) (domain.Zone, error) {
	var createdAt time.Time
	err := r.pool.QueryRow(ctx,
		`UPDATE zones
		    SET code = $2, name = $3, area = ST_GeogFromText($4),
		        version = version + 1, updated_at = now()
		  WHERE id = $1
		  RETURNING created_at`,
		zone.ID, zone.Code, zone.Name, wktPolygon(zone.Ring, zone.Holes),
	).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Zone{}, domain.ErrZoneCodeTaken
		}
		return domain.Zone{}, fmt.Errorf("update zone: %w", err)
	}

	zone.CreatedAt = createdAt
	return zone, nil
}

// FindByID carga la zona con su anillo exterior y sus huecos.
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

// FindContaining devuelve la zona que cubre el punto. Si hay solapamiento
// elige la de menor área (y, a igualdad, la de id menor) para que el resultado
// sea determinista. ST_Covers excluye los puntos dentro de un hueco.
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

// ringPointsQuery devuelve los puntos de todos los anillos de cada zona. En ST_DumpPoints,
// path[1] es el índice del anillo: 1 es el exterior y 2 en adelante, los huecos.
const ringPointsQuery = `SELECT z.id::text, z.code, z.name, z.created_at,
		        d.path[1], ST_Y(d.geom), ST_X(d.geom)
		   FROM zones z
		   CROSS JOIN LATERAL ST_DumpPoints(z.area::geometry) AS d`

// collectZones agrupa las filas de ringPointsQuery por zona, manteniendo el orden de llegada.
// Las filas de una misma zona deben venir juntas, lo que garantizan las consultas que la usan.
func collectZones(rows pgx.Rows) ([]domain.Zone, error) {
	var zones []domain.Zone
	index := map[string]int{}
	for rows.Next() {
		var id, code, name string
		var createdAt time.Time
		var ringIndex int32
		var lat, lng float64
		if err := rows.Scan(&id, &code, &name, &createdAt, &ringIndex, &lat, &lng); err != nil {
			return nil, fmt.Errorf("scan ring point: %w", err)
		}
		i, seen := index[id]
		if !seen {
			zones = append(zones, domain.Zone{ID: id, Code: code, Name: name, CreatedAt: createdAt})
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
