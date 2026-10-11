// Package domain contiene las reglas de negocio de las zonas de reparto.
// No depende de PostgreSQL, PostGIS, gRPC ni del framework HTTP.
package domain

import (
	"math"
	"strings"
	"time"
)

const (
	minVertices   = 4               // trapecio o cuadrado como mínimo; un triángulo no es una zona válida
	minRingPoints = minVertices + 1 // los vértices más el primero repetido al final
	maxCodeLength = 32
	maxNameLength = 120
)

// Coordinate es un punto geográfico en grados decimales (WGS 84).
type Coordinate struct {
	Latitude  float64
	Longitude float64
}

// NewCoordinate valida que el punto esté dentro de los rangos geográficos.
func NewCoordinate(latitude, longitude float64) (Coordinate, error) {
	if math.IsNaN(latitude) || math.IsNaN(longitude) {
		return Coordinate{}, ErrInvalidCoordinate
	}
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return Coordinate{}, ErrInvalidCoordinate
	}
	return Coordinate{Latitude: latitude, Longitude: longitude}, nil
}

// Zone es una región geográfica de reparto: un anillo exterior cerrado y, opcionalmente,
// huecos (dona). Cada hueco es un anillo cerrado estrictamente dentro del exterior.
type Zone struct {
	ID        string
	Code      string
	Name      string
	Ring      []Coordinate
	Holes     [][]Coordinate
	CreatedAt time.Time
}

// NewZone valida una zona sin huecos. El ID y la fecha de creación los asigna la persistencia.
func NewZone(code, name string, ring []Coordinate) (Zone, error) {
	return NewZoneWithHoles(code, name, ring, nil)
}

// NewZoneWithHoles valida una zona con huecos opcionales.
func NewZoneWithHoles(code, name string, ring []Coordinate, holes [][]Coordinate) (Zone, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)

	if code == "" || len(code) > maxCodeLength {
		return Zone{}, ErrInvalidCode
	}
	if name == "" || len(name) > maxNameLength {
		return Zone{}, ErrInvalidName
	}
	if err := validateRing(ring); err != nil {
		return Zone{}, err
	}
	for _, hole := range holes {
		if err := validateRing(hole); err != nil {
			return Zone{}, err
		}
	}
	if err := validateHoles(ring, holes); err != nil {
		return Zone{}, err
	}

	return Zone{
		Code:  code,
		Name:  name,
		Ring:  append([]Coordinate(nil), ring...),
		Holes: copyRings(holes),
	}, nil
}

func copyRings(rings [][]Coordinate) [][]Coordinate {
	out := make([][]Coordinate, 0, len(rings))
	for _, r := range rings {
		out = append(out, append([]Coordinate(nil), r...))
	}
	return out
}

func validateRing(ring []Coordinate) error {
	if len(ring) < minRingPoints {
		return ErrInvalidRing
	}
	if ring[0] != ring[len(ring)-1] {
		return ErrRingNotClosed
	}
	for _, p := range ring {
		if _, err := NewCoordinate(p.Latitude, p.Longitude); err != nil {
			return err
		}
	}
	if !isConvexPolygon(ring[:len(ring)-1]) {
		return ErrNotConvex
	}
	return nil
}
