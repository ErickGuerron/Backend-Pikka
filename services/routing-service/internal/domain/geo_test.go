package domain

import (
	"math"
	"testing"
)

func TestDistanceMetersMatchesKnownSpacing(t *testing.T) {
	// Un grado de latitud son unos 111.2 km sobre la superficie esferica de referencia.
	got := DistanceMeters(Point{Latitude: 0, Longitude: 0}, Point{Latitude: 1, Longitude: 0})
	if math.Abs(got-111_195) > 100 {
		t.Fatalf("DistanceMeters one degree of latitude = %.0f m, want about 111195", got)
	}
}

func TestDistanceMetersIsZeroForTheSamePoint(t *testing.T) {
	p := Point{Latitude: -0.18, Longitude: -78.48}
	if got := DistanceMeters(p, p); got != 0 {
		t.Fatalf("DistanceMeters same point = %v, want 0", got)
	}
}

func TestCentroidOfSquareIsItsCenter(t *testing.T) {
	ring := []Point{
		{Latitude: 1, Longitude: 1},
		{Latitude: 1, Longitude: 3},
		{Latitude: 3, Longitude: 3},
		{Latitude: 3, Longitude: 1},
	}
	got, ok := Centroid(ring)
	if !ok || got != (Point{Latitude: 2, Longitude: 2}) {
		t.Fatalf("Centroid = %+v ok=%v, want {2 2} true", got, ok)
	}
}

func TestCentroidIgnoresTheClosingPoint(t *testing.T) {
	// Un anillo cerrado repite el primer punto al final: no debe pesar dos veces.
	ring := []Point{
		{Latitude: 1, Longitude: 1},
		{Latitude: 1, Longitude: 3},
		{Latitude: 3, Longitude: 3},
		{Latitude: 3, Longitude: 1},
		{Latitude: 1, Longitude: 1},
	}
	got, ok := Centroid(ring)
	if !ok || got != (Point{Latitude: 2, Longitude: 2}) {
		t.Fatalf("Centroid closed ring = %+v ok=%v, want {2 2} true", got, ok)
	}
}

func TestCentroidOfEmptyRingIsNotOK(t *testing.T) {
	if _, ok := Centroid(nil); ok {
		t.Fatal("Centroid(nil) ok = true, want false")
	}
}
