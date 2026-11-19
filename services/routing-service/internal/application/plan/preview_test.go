package plan

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

func TestPreviewProposesTheSamePlanAsRunWithoutChangingAnything(t *testing.T) {
	pending := []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
		{OrderID: "o4", ZoneID: "B"},
	}
	zones := fakeZones{capacity: map[string]int{"A": 2, "B": 5}}

	ran := &fakeOrders{failAt: -1, pending: pending}
	want, err := newUseCase(ran, zones, &fakeDrivers{available: []string{"d1", "d2", "d3"}, reserveFailAt: -1}).Run(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	orders := &fakeOrders{failAt: -1, pending: pending}
	drivers := &fakeDrivers{available: []string{"d1", "d2", "d3"}, reserveFailAt: -1}
	got, err := newUseCase(orders, zones, drivers).Preview(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if len(orders.assigned) != 0 || len(drivers.reserved) != 0 {
		t.Fatalf("preview changed state: assigned %+v reserved %+v", orders.assigned, drivers.reserved)
	}
	if len(got.Routes) != len(want.Routes) {
		t.Fatalf("routes = %d, want %d", len(got.Routes), len(want.Routes))
	}
	for i, route := range got.Routes {
		if route.RouteID != want.Routes[i].RouteID || route.DriverID != want.Routes[i].DriverID || route.OrderCount != want.Routes[i].OrderCount {
			t.Fatalf("route %d = %+v, want %+v", i, route, want.Routes[i])
		}
	}
	if !slices.EqualFunc(got.Assigned, want.Assigned, func(a, b domain.Route) bool { return a.ID == b.ID && len(a.Stops) == len(b.Stops) }) {
		t.Fatalf("planned stops differ: %+v vs %+v", got.Assigned, want.Assigned)
	}
}

func TestPreviewLeavesTheDriverEmptyWhenNoneIsLeft(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 1}}

	got, err := newUseCase(orders, zones, drivers).Preview(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Preview must not fail for a route without driver: %v", err)
	}
	if got.Routes[0].DriverID != "d1" || got.Routes[1].DriverID != "" {
		t.Fatalf("drivers = %q, %q; want d1 then empty", got.Routes[0].DriverID, got.Routes[1].DriverID)
	}
}

func TestPreviewRejectsAnInvalidDateAndSurfacesLoadErrors(t *testing.T) {
	uc := newUseCase(&fakeOrders{failAt: -1}, fakeZones{}, &fakeDrivers{reserveFailAt: -1})
	if _, err := uc.Preview(context.Background(), "15/10/2026"); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("err = %v, want ErrInvalidDate", err)
	}

	boom := errors.New("orders down")
	uc = newUseCase(&fakeOrders{failAt: -1, pendErr: boom}, fakeZones{}, &fakeDrivers{reserveFailAt: -1})
	if _, err := uc.Preview(context.Background(), "2026-10-15"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the load error", err)
	}
}
