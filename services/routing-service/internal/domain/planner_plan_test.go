package domain_test

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// assertPartition checks every input order appears exactly once and no route exceeds capacity.
func assertPartition(t *testing.T, routes []domain.Route, orders []domain.PlannableOrder, capacity int) {
	t.Helper()
	seen := map[string]int{}
	for _, r := range routes {
		if len(r.Stops) == 0 || len(r.Stops) > capacity {
			t.Fatalf("route %s has %d stops, want 1..%d", r.ID, len(r.Stops), capacity)
		}
		for _, s := range r.Stops {
			seen[s.OrderID]++
		}
	}
	if len(seen) != len(orders) {
		t.Fatalf("planned %d distinct orders, want %d", len(seen), len(orders))
	}
	for _, o := range orders {
		if seen[o.OrderID] != 1 {
			t.Fatalf("order %s appears %d times, want 1", o.OrderID, seen[o.OrderID])
		}
	}
}

// uniqueZoneOrders builds n orders in zone with ids unique across zones (order ids are global).
func uniqueZoneOrders(zone string, n int) []domain.PlannableOrder {
	out := make([]domain.PlannableOrder, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, ord(zone, fmt.Sprintf("%s-o%02d", zone, i)))
	}
	return out
}

// Plan: límites de capacidad por ruta.
func TestPlan_Planner_CapacityBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		n, cap     int
		wantRoutes int
	}{
		{"0 orders", 0, 5, 0},
		{"N == cap", 5, 5, 1},
		{"N == cap+1", 6, 5, 2},
		{"cap == 1", 4, 1, 4},
		{"N == 2*cap", 10, 5, 2},
		{"N == 3*cap", 9, 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orders := zoneOrders("z1", tt.n)
			routes, err := domain.ZoneCapacityPlanner{}.Plan(orders, tt.cap)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(routes) != tt.wantRoutes {
				t.Fatalf("got %d routes, want %d", len(routes), tt.wantRoutes)
			}
			assertPartition(t, routes, orders, tt.cap)
		})
	}
}

// Plan: pedido de último momento. Adding one order to the same input keeps a valid partition.
func TestPlan_Planner_LateOrderKeepsValidPartition(t *testing.T) {
	const capacity = 3
	base := append(uniqueZoneOrders("z1", 6), uniqueZoneOrders("z2", 2)...)
	late := ord("z1", "o99")
	planner := domain.ZoneCapacityPlanner{}

	before, err := planner.Plan(base, capacity)
	if err != nil {
		t.Fatal(err)
	}
	assertPartition(t, before, base, capacity)

	withLate := append(append([]domain.PlannableOrder{}, base...), late)
	after, err := planner.Plan(withLate, capacity)
	if err != nil {
		t.Fatal(err)
	}
	assertPartition(t, after, withLate, capacity)
	if len(after) != len(before)+1 {
		t.Fatalf("z1 was full (6 = 2*cap): got %d routes, want %d", len(after), len(before)+1)
	}
}

// The planner is stateless: concurrent callers with shuffled inputs get identical output.
func TestPlan_Planner_ConcurrentCallersAreDeterministic(t *testing.T) {
	const callers, capacity = 8, 4
	var orders []domain.PlannableOrder
	for z := 1; z <= 3; z++ {
		orders = append(orders, uniqueZoneOrders(fmt.Sprintf("z%d", z), 9)...)
	}
	planner := domain.ZoneCapacityPlanner{}
	want, err := planner.Plan(orders, capacity)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make([][]domain.Route, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := make([]domain.PlannableOrder, len(orders))
			for j := range orders { // reversed copy: order of input must not matter
				in[j] = orders[len(orders)-1-j]
			}
			<-start
			results[i], errs[i] = planner.Plan(in, capacity)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if !reflect.DeepEqual(results[i], want) {
			t.Fatalf("caller %d result differs from the reference plan", i)
		}
	}
}
