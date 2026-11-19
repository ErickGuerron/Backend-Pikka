package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

var errGateway = errors.New("gateway down")

// zonesCenterFails falla al leer el centro de la zona indicada.
type zonesCenterFails struct {
	fakeZones
	zone string
	err  error
}

func (f zonesCenterFails) ZoneCenter(ctx context.Context, zoneID string) (domain.Point, bool, error) {
	if zoneID == f.zone {
		return domain.Point{}, false, f.err
	}
	return f.fakeZones.ZoneCenter(ctx, zoneID)
}

// failingPlanner rechaza cualquier zona, para comprobar que el error del planificador llega con la zona.
type failingPlanner struct{ err error }

func (f failingPlanner) Plan([]domain.PlannableOrder, int) ([]domain.Route, error) {
	return nil, f.err
}

func TestGeneratePendingOrdersFailureIsWrappedAndReservesNothing(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pendErr: errGateway}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1}

	_, err := newUseCase(orders, fakeZones{}, drivers).Run(context.Background(), "2026-10-25")

	if !errors.Is(err, errGateway) || !strings.Contains(err.Error(), "load pending orders") {
		t.Fatalf("err = %v, want the gateway error wrapped as a load failure", err)
	}
	if len(drivers.reserved) != 0 {
		t.Fatal("un fallo al cargar pedidos no reserva conductores")
	}
}

func TestGenerateZoneCenterFailureStopsBeforeReserving(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1}
	zones := zonesCenterFails{
		fakeZones: fakeZones{capacity: map[string]int{"A": 5}},
		zone:      "A",
		err:       errGateway,
	}

	_, err := New(orders, zones, drivers, fakeLocations{}, domain.ZoneCapacityPlanner{}).Run(context.Background(), "2026-10-25")

	if !errors.Is(err, errGateway) || !strings.Contains(err.Error(), "zone A center") {
		t.Fatalf("err = %v, want the center failure of zone A", err)
	}
	if len(drivers.reserved) != 0 || len(orders.assigned) != 0 {
		t.Fatal("un fallo al leer el centro no reserva ni asigna nada")
	}
}

func TestGeneratePlannerFailureNamesTheZone(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "B"}}}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"B": 5}}

	_, err := New(orders, zones, drivers, fakeLocations{}, failingPlanner{err: errGateway}).Run(context.Background(), "2026-10-25")

	if !errors.Is(err, errGateway) || !strings.Contains(err.Error(), "plan zone B") {
		t.Fatalf("err = %v, want the planner failure of zone B", err)
	}
	if len(drivers.reserved) != 0 {
		t.Fatal("un fallo de planificación no reserva conductores")
	}
}

func TestGenerateDriverLookupFailureIsReported(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{findErr: errGateway, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-25")

	if !errors.Is(err, errGateway) || !strings.Contains(err.Error(), "find driver for route") {
		t.Fatalf("err = %v, want the lookup failure", err)
	}
	if len(orders.assigned) != 0 {
		t.Fatal("sin conductor no se asigna ningún pedido")
	}
}

func TestEqualDistanceDriversKeepDriverServiceOrder(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"primero", "segundo"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}, centers: map[string]domain.Point{"A": zoneCenter}}
	// Ambos conductores están exactamente en el mismo punto: la distancia empata.
	locations := fakeLocations{at: map[string]domain.Point{"primero": nearZone, "segundo": nearZone}}

	if _, err := New(orders, zones, drivers, locations, domain.ZoneCapacityPlanner{}).Run(context.Background(), "2026-10-25"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(drivers.reserved) != 1 || drivers.reserved[0].driverID != "primero" {
		t.Fatalf("reserved = %+v, want the first driver when distances tie", drivers.reserved)
	}
}

func TestEqualDistanceUnknownDriversKeepOrder(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"x", "y"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}, centers: map[string]domain.Point{"A": zoneCenter}}

	if _, err := New(orders, zones, drivers, fakeLocations{}, domain.ZoneCapacityPlanner{}).Run(context.Background(), "2026-10-25"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(drivers.reserved) != 1 || drivers.reserved[0].driverID != "x" {
		t.Fatalf("reserved = %+v, want the first driver when no position is known", drivers.reserved)
	}
}
