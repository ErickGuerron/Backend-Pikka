// Package domain contiene las reglas de negocio de las zonas de reparto.
// No depende de PostgreSQL, PostGIS, gRPC ni del framework HTTP.
package domain

import (
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	minVertices     = 4               // trapecio o cuadrado como mínimo; un triángulo no es una zona válida
	minRingPoints   = minVertices + 1 // los vértices más el primero repetido al final
	maxCodeLength   = 32
	minNameLength   = 3
	maxNameLength   = 120
	minReasonLength = 5
	maxReasonLength = 300
)

// Estados de una zona. Una zona inactiva no clasifica puntos ni valida pedidos, pero se conserva.
const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
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

// Zone es una región geográfica de reparto: un anillo exterior cerrado y, opcionalmente, huecos
// (dona). Cada hueco es un anillo cerrado estrictamente dentro del exterior.
type Zone struct {
	ID           string
	Code         string
	Name         string
	Ring         []Coordinate
	Holes        [][]Coordinate
	Status       string
	StatusReason string
	Settings     Settings
	// Version sube en cada cambio. Al editar, el cliente envía la versión que vio para evitar
	// sobrescribir cambios ajenos.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// NewZoneWithHoles valida una zona con huecos opcionales y su configuración. Una zona nueva nace
// activa. El ID, la fecha y los autores los asigna la persistencia.
func NewZoneWithHoles(code, name string, ring []Coordinate, holes [][]Coordinate, settings Settings) (Zone, error) {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > maxCodeLength {
		return Zone{}, ErrInvalidCode
	}
	trimmedName, err := ValidateName(name)
	if err != nil {
		return Zone{}, err
	}
	if err := ValidateGeometry(ring, holes); err != nil {
		return Zone{}, err
	}
	normalized, err := ValidateSettings(settings)
	if err != nil {
		return Zone{}, err
	}

	return Zone{
		Code:     code,
		Name:     trimmedName,
		Ring:     append([]Coordinate(nil), ring...),
		Holes:    copyRings(holes),
		Status:   StatusActive,
		Settings: normalized,
		Version:  1,
	}, nil
}

// zoneNamePattern: letras (con acentos), números, espacios y . - & ( ) '.
var zoneNamePattern = regexp.MustCompile(`^[\p{L}\p{N} .\-&()']+$`)

// ValidateName devuelve el nombre sin espacios sobrantes, o ErrInvalidName si no es válido.
// Un nombre tiene entre 3 y 120 caracteres y solo usa letras, números, espacios y . - & ( ) '.
func ValidateName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	length := utf8.RuneCountInString(trimmed)
	if length < minNameLength || length > maxNameLength || !zoneNamePattern.MatchString(trimmed) {
		return "", ErrInvalidName
	}
	return trimmed, nil
}

// ValidateGeometry valida el anillo exterior y los huecos: cerrados, convexos, con al menos cuatro
// vértices, y cada hueco dentro del exterior sin tocar otro.
func ValidateGeometry(ring []Coordinate, holes [][]Coordinate) error {
	if err := validateRing(ring); err != nil {
		return err
	}
	for _, hole := range holes {
		if err := validateRing(hole); err != nil {
			return err
		}
	}
	return validateHoles(ring, holes)
}

// ValidateReason devuelve el motivo sin espacios sobrantes. Todo cambio de estado o borrado lo exige.
func ValidateReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if len(trimmed) < minReasonLength || len(trimmed) > maxReasonLength {
		return "", ErrReasonRequired
	}
	return trimmed, nil
}

// ValidStatus indica si el estado es uno de los que admite una zona.
func ValidStatus(status string) bool {
	return status == StatusActive || status == StatusInactive
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
