package domain

import "errors"

// ErrNoRoute means no road route connects the two points (off the loaded map, or unreachable).
var ErrNoRoute = errors.New("no road route between the points")

// Point is a WGS 84 coordinate used for routing.
type Point struct {
	Latitude  float64
	Longitude float64
}

// Estimate is the road distance and driving time between two points.
type Estimate struct {
	DistanceMeters  float64
	DurationSeconds float64
}
