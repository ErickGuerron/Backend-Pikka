package domain

import (
	"errors"
	"math"
)

// earthRadiusMeters es el radio medio de la Tierra usado por la fórmula de Haversine.
const earthRadiusMeters = 6371000

// ErrInvalidCoordinates indica una latitud o longitud fuera de rango.
var ErrInvalidCoordinates = errors.New("coordinates out of range")

// GeoPoint es una ubicación en grados decimales (WGS 84).
type GeoPoint struct {
	Latitude  float64
	Longitude float64
}

// Validate comprueba que las coordenadas existan y estén dentro de rango.
func (p GeoPoint) Validate() error {
	if math.IsNaN(p.Latitude) || math.IsNaN(p.Longitude) {
		return ErrInvalidCoordinates
	}
	if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
		return ErrInvalidCoordinates
	}
	return nil
}

// DistanceMeters devuelve la distancia en metros sobre la superficie terrestre (Haversine).
func DistanceMeters(a, b GeoPoint) float64 {
	lat1 := a.Latitude * math.Pi / 180
	lat2 := b.Latitude * math.Pi / 180
	dLat := (b.Latitude - a.Latitude) * math.Pi / 180
	dLon := (b.Longitude - a.Longitude) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusMeters * math.Asin(math.Min(1, math.Sqrt(h)))
}
