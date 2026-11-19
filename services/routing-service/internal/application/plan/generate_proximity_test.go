package plan

import (
	"context"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// fakeLocations answers the last fresh position of each driver. A driver missing from at has no fresh fix.
type fakeLocations struct {
	at  map[string]domain.Point
	err error
}

func (f fakeLocations) LastPosition(_ context.Context, driverID string) (domain.Point, bool, error) {
	if f.err != nil {
		return domain.Point{}, false, f.err
	}
	p, ok := f.at[driverID]
	return p, ok, nil
}

// ZoneCenter gives the zone's center from the centers map; a zone missing from it has no geometry.
func (f fakeZones) ZoneCenter(_ context.Context, zoneID string) (domain.Point, bool, error) {
	p, ok := f.centers[zoneID]
	return p, ok, nil
}

var (
	zoneCenter = domain.Point{Latitude: -1.24, Longitude: -78.62}
	nearZone   = domain.Point{Latitude: -1.2405, Longitude: -78.62} // ~55 m del centro
	farZone    = domain.Point{Latitude: -1.10, Longitude: -78.40}   // ~25 km del centro
)

func TestRouteGoesToTheDriverNearestToItsZone(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"lejos", "cerca"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}, centers: map[string]domain.Point{"A": zoneCenter}}
	locations := fakeLocations{at: map[string]domain.Point{"lejos": farZone, "cerca": nearZone}}

	uc := New(orders, zones, drivers, locations, domain.ZoneCapacityPlanner{})
	if _, err := uc.Run(context.Background(), "2026-10-25"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(drivers.reserved) != 1 || drivers.reserved[0].driverID != "cerca" {
		t.Fatalf("reserved = %+v, want the nearest driver \"cerca\"", drivers.reserved)
	}
}

func TestDriverWithoutFreshPositionIsTriedLast(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"sin-gps", "cerca"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}, centers: map[string]domain.Point{"A": zoneCenter}}
	locations := fakeLocations{at: map[string]domain.Point{"cerca": nearZone}}

	uc := New(orders, zones, drivers, locations, domain.ZoneCapacityPlanner{})
	if _, err := uc.Run(context.Background(), "2026-10-25"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if drivers.reserved[0].driverID != "cerca" {
		t.Fatalf("reserved %q, want \"cerca\": a driver with a fresh fix beats one without", drivers.reserved[0].driverID)
	}
}

func TestWithoutZoneCenterKeepsTheDriverOrder(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"primero", "cerca"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}} // sin geometria para la zona
	locations := fakeLocations{at: map[string]domain.Point{"cerca": nearZone}}

	uc := New(orders, zones, drivers, locations, domain.ZoneCapacityPlanner{})
	if _, err := uc.Run(context.Background(), "2026-10-25"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if drivers.reserved[0].driverID != "primero" {
		t.Fatalf("reserved %q, want \"primero\": without a zone center there is no distance to rank by", drivers.reserved[0].driverID)
	}
}

func TestEachRouteTakesTheNearestRemainingDriver(t *testing.T) {
	// Dos rutas en la misma zona (capacidad 1): cada una toma el repartidor mas cercano que quede libre.
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"lejos", "cerca"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 1}, centers: map[string]domain.Point{"A": zoneCenter}}
	locations := fakeLocations{at: map[string]domain.Point{"lejos": farZone, "cerca": nearZone}}

	uc := New(orders, zones, drivers, locations, domain.ZoneCapacityPlanner{})
	if _, err := uc.Run(context.Background(), "2026-10-25"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(drivers.reserved) != 2 || drivers.reserved[0].driverID != "cerca" || drivers.reserved[1].driverID != "lejos" {
		t.Fatalf("reserved = %+v, want cerca then lejos", drivers.reserved)
	}
}

func TestLocationErrorStopsBeforeReserving(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"cerca"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}, centers: map[string]domain.Point{"A": zoneCenter}}
	locations := fakeLocations{err: context.DeadlineExceeded}

	uc := New(orders, zones, drivers, locations, domain.ZoneCapacityPlanner{})
	if _, err := uc.Run(context.Background(), "2026-10-25"); err == nil {
		t.Fatal("Run without error, want the tracking failure")
	}
	if len(drivers.reserved) != 0 {
		t.Fatalf("reserved %+v, want nothing reserved when positions cannot be read", drivers.reserved)
	}
}
