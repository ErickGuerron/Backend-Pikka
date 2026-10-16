package domain

import (
	"errors"
	"math"
)

// Errors returned when estimating the road distance between two points.
var (
	ErrInvalidPoint = errors.New("latitude must be in [-90, 90] and longitude in [-180, 180], both finite")
	ErrNoRoute      = errors.New("no road route between the points")
)

// Point is a WGS 84 coordinate.
type Point struct {
	Latitude  float64
	Longitude float64
}

// Valid reports whether the point is a finite WGS 84 coordinate.
func (p Point) Valid() bool {
	return p.Latitude >= -90 && p.Latitude <= 90 &&
		p.Longitude >= -180 && p.Longitude <= 180 &&
		!math.IsNaN(p.Latitude) && !math.IsNaN(p.Longitude) &&
		!math.IsInf(p.Latitude, 0) && !math.IsInf(p.Longitude, 0)
}

// Estimate is the road distance and driving time between two points.
type Estimate struct {
	DistanceMeters  float64
	DurationSeconds float64
}
