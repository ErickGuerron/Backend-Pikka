package domain

import "math"

// earthRadiusMeters is the mean Earth radius used by the haversine formula.
const earthRadiusMeters = 6_371_000

// DistanceMeters is the straight-line distance between two points on the sphere (haversine).
// It ranks drivers by how close they are to a zone; it is not a road distance.
func DistanceMeters(a, b Point) float64 {
	rad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := rad(b.Latitude - a.Latitude)
	dLon := rad(b.Longitude - a.Longitude)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(a.Latitude))*math.Cos(rad(b.Latitude))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusMeters * math.Asin(math.Min(1, math.Sqrt(h)))
}

// Centroid is the average of the ring vertices, the point used to place a zone on the map.
// A closing point that repeats the first vertex is ignored. It reports false for an empty ring.
func Centroid(ring []Point) (Point, bool) {
	if len(ring) > 1 && ring[0] == ring[len(ring)-1] {
		ring = ring[:len(ring)-1]
	}
	if len(ring) == 0 {
		return Point{}, false
	}
	var sumLat, sumLon float64
	for _, p := range ring {
		sumLat += p.Latitude
		sumLon += p.Longitude
	}
	n := float64(len(ring))
	return Point{Latitude: sumLat / n, Longitude: sumLon / n}, true
}
