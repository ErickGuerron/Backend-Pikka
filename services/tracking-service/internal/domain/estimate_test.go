package domain

import "testing"

func TestPointValidAcceptsOnlyWGS84Ranges(t *testing.T) {
	tests := map[string]struct {
		point Point
		want  bool
	}{
		"origin":               {Point{0, 0}, true},
		"north pole edge":      {Point{90, 0}, true},
		"south pole edge":      {Point{-90, 0}, true},
		"antimeridian edge":    {Point{0, 180}, true},
		"latitude above 90":    {Point{90.0001, 0}, false},
		"latitude below -90":   {Point{-91, 0}, false},
		"longitude above 180":  {Point{0, 180.5}, false},
		"longitude below -180": {Point{0, -181}, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.point.Valid(); got != tt.want {
				t.Fatalf("Valid(%+v) = %v, want %v", tt.point, got, tt.want)
			}
		})
	}
}
