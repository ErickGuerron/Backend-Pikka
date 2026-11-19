package plan

import (
	"context"
	"errors"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

var newOrder = domain.PendingOrder{OrderID: "n1", ZoneID: "A", Address: "Calle Nueva 1", DeliveryDate: "2026-10-15"}

func runningRoute() []domain.RouteOrder {
	return []domain.RouteOrder{
		{OrderID: "o1", ZoneID: "A", Sequence: 1, Open: false},
		{OrderID: "o2", ZoneID: "A", Sequence: 2, Open: true},
		{OrderID: "o3", ZoneID: "A", Sequence: 3, Open: true},
	}
}

func addCase(onRoute []domain.RouteOrder, capacity int) (*AddToRoute, *fakeOrders) {
	orders := &fakeOrders{failAt: -1, findable: []domain.PendingOrder{newOrder}, onRoute: onRoute}
	return NewAddToRoute(orders, fakeZones{capacity: map[string]int{"A": capacity}}), orders
}

func TestAddToRouteAppendsTheOrderAsTheLastStop(t *testing.T) {
	uc, orders := addCase(runningRoute(), 10)

	stop, err := uc.Run(context.Background(), "2026-10-15-A-1", "n1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stop.Sequence != 4 || stop.Address != "Calle Nueva 1" {
		t.Fatalf("stop = %+v, want sequence 4 and the order's address", stop)
	}
	if len(orders.assigned) != 1 || orders.assigned[0].routeID != "2026-10-15-A-1" ||
		len(orders.assigned[0].stops) != 1 || orders.assigned[0].stops[0].OrderID != "n1" || orders.assigned[0].stops[0].Sequence != 4 {
		t.Fatalf("assigned = %+v, want only n1 as stop 4 of the route", orders.assigned)
	}
}

func TestAddToRouteRejectsWhatCannotBePlacedAndAssignsNothing(t *testing.T) {
	finished := []domain.RouteOrder{{OrderID: "o1", ZoneID: "A", Sequence: 1, Open: false}}
	otherZone := []domain.RouteOrder{{OrderID: "o1", ZoneID: "B", Sequence: 1, Open: true}}
	tests := []struct {
		name     string
		route    string
		order    string
		onRoute  []domain.RouteOrder
		capacity int
		want     error
	}{
		{"blank route", " ", "n1", runningRoute(), 10, ErrBlankID},
		{"blank order", "r", "", runningRoute(), 10, ErrBlankID},
		{"order not pending or unknown", "r", "zzz", runningRoute(), 10, ErrOrderNotPending},
		{"route without orders that day", "r", "n1", nil, 10, ErrRouteNotFound},
		{"route of another zone", "r", "n1", otherZone, 10, ErrZoneMismatch},
		{"route already finished", "r", "n1", finished, 10, ErrRouteFinished},
		{"route at capacity", "r", "n1", runningRoute(), 3, ErrRouteFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, orders := addCase(tt.onRoute, tt.capacity)
			if _, err := uc.Run(context.Background(), tt.route, tt.order); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if len(orders.assigned) != 0 {
				t.Fatalf("assigned %+v, want nothing", orders.assigned)
			}
		})
	}
}

func TestAddToRouteSurfacesUpstreamFailures(t *testing.T) {
	boom := errors.New("order service down")

	uc, orders := addCase(runningRoute(), 10)
	orders.lookupErr = boom
	if _, err := uc.Run(context.Background(), "r", "n1"); !errors.Is(err, boom) {
		t.Fatalf("lookup err = %v, want %v", err, boom)
	}

	uc, orders = addCase(runningRoute(), 10)
	orders.failAt, orders.assignErr = 0, boom
	if _, err := uc.Run(context.Background(), "r", "n1"); !errors.Is(err, boom) {
		t.Fatalf("assign err = %v, want %v", err, boom)
	}
}
