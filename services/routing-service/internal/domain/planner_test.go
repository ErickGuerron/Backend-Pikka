package domain_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

var _ domain.RoutePlanner = domain.ZoneCapacityPlanner{}

func ord(zone, id string) domain.PlannableOrder {
	return domain.PlannableOrder{OrderID: id, ZoneID: zone}
}

// zoneOrders builds n orders o01..onn in the given zone.
func zoneOrders(zone string, n int) []domain.PlannableOrder {
	out := make([]domain.PlannableOrder, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, ord(zone, fmt.Sprintf("o%02d", i)))
	}
	return out
}

// route builds the expected route for a zone with the given order ids.
func route(zone string, n int, ids ...string) domain.Route {
	stops := make([]domain.RouteStop, 0, len(ids))
	for i, id := range ids {
		stops = append(stops, domain.RouteStop{OrderID: id, ZoneID: zone, Sequence: i + 1})
	}
	return domain.Route{ID: fmt.Sprintf("%s#%d", zone, n), ZoneID: zone, Stops: stops}
}

func TestPlanValidationErrors(t *testing.T) {
	valid := zoneOrders("z1", 2)
	tests := []struct {
		name   string
		orders []domain.PlannableOrder
		cap    int
		want   error
	}{
		{"zero capacity", valid, 0, domain.ErrInvalidCapacity},
		{"negative capacity", valid, -1, domain.ErrInvalidCapacity},
		{"empty order id", []domain.PlannableOrder{ord("z1", "")}, 2, domain.ErrInvalidOrder},
		{"whitespace order id", []domain.PlannableOrder{ord("z1", "  ")}, 2, domain.ErrInvalidOrder},
		{"empty zone id", []domain.PlannableOrder{ord("", "o1")}, 2, domain.ErrInvalidOrder},
		{"whitespace zone id", []domain.PlannableOrder{ord(" \t", "o1")}, 2, domain.ErrInvalidOrder},
		{"duplicate same zone", []domain.PlannableOrder{ord("z1", "o1"), ord("z1", "o1")}, 2, domain.ErrDuplicateOrder},
		{"duplicate different zone", []domain.PlannableOrder{ord("z1", "o1"), ord("z2", "o1")}, 2, domain.ErrDuplicateOrder},
		{"capacity error wins over bad orders", []domain.PlannableOrder{ord("", "")}, 0, domain.ErrInvalidCapacity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := domain.ZoneCapacityPlanner{}.Plan(tt.orders, tt.cap)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Plan() error = %v, want %v", err, tt.want)
			}
			if routes != nil {
				t.Fatalf("Plan() routes = %v, want nil on error", routes)
			}
		})
	}
}

func TestPlanPartition(t *testing.T) {
	tests := []struct {
		name   string
		orders []domain.PlannableOrder
		cap    int
		want   []domain.Route
	}{
		{"capacity 1", zoneOrders("z1", 3), 1, []domain.Route{
			route("z1", 1, "o01"), route("z1", 2, "o02"), route("z1", 3, "o03"),
		}},
		{"capacity 2", zoneOrders("z1", 4), 2, []domain.Route{
			route("z1", 1, "o01", "o02"), route("z1", 2, "o03", "o04"),
		}},
		{"capacity 4 with remainder", zoneOrders("z1", 9), 4, []domain.Route{
			route("z1", 1, "o01", "o02", "o03", "o04"),
			route("z1", 2, "o05", "o06", "o07", "o08"),
			route("z1", 3, "o09"),
		}},
		{"capacity 50 single route", zoneOrders("z1", 10), 50, []domain.Route{
			route("z1", 1, "o01", "o02", "o03", "o04", "o05", "o06", "o07", "o08", "o09", "o10"),
		}},
		{"uneven remainder restarts sequence", zoneOrders("z1", 5), 2, []domain.Route{
			route("z1", 1, "o01", "o02"), route("z1", 2, "o03", "o04"), route("z1", 3, "o05"),
		}},
		{"multiple zones ordered by zone id", []domain.PlannableOrder{
			ord("z2", "b"), ord("z1", "d"), ord("z2", "a"), ord("z1", "c"),
		}, 10, []domain.Route{
			route("z1", 1, "c", "d"), route("z2", 1, "a", "b"),
		}},
		{"route counter resets per zone", []domain.PlannableOrder{
			ord("z1", "a"), ord("z1", "b"), ord("z1", "c"), ord("z2", "d"), ord("z2", "e"),
		}, 2, []domain.Route{
			route("z1", 1, "a", "b"), route("z1", 2, "c"), route("z2", 1, "d", "e"),
		}},
		{"empty input", []domain.PlannableOrder{}, 3, []domain.Route{}},
		{"nil input", nil, 3, []domain.Route{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ZoneCapacityPlanner{}.Plan(tt.orders, tt.cap)
			if err != nil {
				t.Fatalf("Plan() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Plan() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPlanCapacityIsInjected(t *testing.T) {
	orders := zoneOrders("z1", 4)
	planner := domain.ZoneCapacityPlanner{}

	small, err := planner.Plan(orders, 1)
	if err != nil {
		t.Fatalf("Plan(cap 1) unexpected error: %v", err)
	}
	large, err := planner.Plan(orders, 4)
	if err != nil {
		t.Fatalf("Plan(cap 4) unexpected error: %v", err)
	}
	if len(small) != 4 || len(large) != 1 {
		t.Fatalf("routes for cap 1 and 4 = %d and %d, want 4 and 1", len(small), len(large))
	}
}

// assertInvariants checks the partition rules every plan must satisfy.
func assertInvariants(t *testing.T, in []domain.PlannableOrder, out []domain.Route, capacity int) {
	t.Helper()
	zoneOf := make(map[string]string, len(in))
	for _, o := range in {
		zoneOf[o.OrderID] = o.ZoneID
	}
	seen := make(map[string]int, len(in))
	for _, r := range out {
		if len(r.Stops) == 0 || len(r.Stops) > capacity {
			t.Errorf("route %s has %d stops, want 1..%d", r.ID, len(r.Stops), capacity)
		}
		for i, s := range r.Stops {
			seen[s.OrderID]++
			if s.ZoneID != r.ZoneID || zoneOf[s.OrderID] != r.ZoneID {
				t.Errorf("route %s stop %s is in zone %q, want %q", r.ID, s.OrderID, s.ZoneID, r.ZoneID)
			}
			if s.Sequence != i+1 {
				t.Errorf("route %s stop %s sequence = %d, want %d", r.ID, s.OrderID, s.Sequence, i+1)
			}
		}
	}
	if len(seen) != len(in) {
		t.Errorf("planned %d distinct orders, want %d", len(seen), len(in))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("order %s appears %d times, want 1", id, n)
		}
	}
}

func TestPlanInvariantsMixedInput(t *testing.T) {
	in := []domain.PlannableOrder{
		ord("z2", "o3"), ord("z1", "o1"), ord("z2", "o5"), ord("z1", "o2"), ord("z2", "o4"),
	}
	out, err := domain.ZoneCapacityPlanner{}.Plan(in, 2)
	if err != nil {
		t.Fatalf("Plan() unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d routes, want 3", len(out))
	}
	assertInvariants(t, in, out, 2)
}

func TestPlanIsDeterministicForAnyInputOrder(t *testing.T) {
	base := []domain.PlannableOrder{
		ord("z2", "o3"), ord("z1", "o1"), ord("z2", "o5"), ord("z1", "o2"), ord("z2", "o4"),
	}
	reversed := make([]domain.PlannableOrder, len(base))
	rotated := make([]domain.PlannableOrder, len(base))
	for i, o := range base {
		reversed[len(base)-1-i] = o
		rotated[(i+2)%len(base)] = o
	}

	planner := domain.ZoneCapacityPlanner{}
	want, err := planner.Plan(base, 2)
	if err != nil {
		t.Fatalf("Plan() unexpected error: %v", err)
	}
	for name, in := range map[string][]domain.PlannableOrder{"reversed": reversed, "rotated": rotated} {
		got, err := planner.Plan(in, 2)
		if err != nil {
			t.Fatalf("Plan(%s) unexpected error: %v", name, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Plan(%s) = %+v, want %+v", name, got, want)
		}
	}
}

func TestPlanDoesNotMutateInput(t *testing.T) {
	in := []domain.PlannableOrder{ord("z2", "b"), ord("z1", "c"), ord("z1", "a")}
	snapshot := append([]domain.PlannableOrder(nil), in...)

	if _, err := (domain.ZoneCapacityPlanner{}).Plan(in, 2); err != nil {
		t.Fatalf("Plan() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(in, snapshot) {
		t.Fatalf("input mutated: got %+v, want %+v", in, snapshot)
	}
}
