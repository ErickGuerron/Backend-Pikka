package domain_test

import (
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

func TestRouteIDFormat(t *testing.T) {
	orders := []domain.PlannableOrder{
		{OrderID: "o-1", ZoneID: "z-1"},
		{OrderID: "o-2", ZoneID: "z-1"},
		{OrderID: "o-3", ZoneID: "z-1"},
		{OrderID: "o-4", ZoneID: "z-2"},
	}

	routes, err := domain.ZoneCapacityPlanner{}.Plan(orders, 1)
	if err != nil {
		t.Fatalf("Plan() unexpected error: %v", err)
	}

	want := []string{"z-1#1", "z-1#2", "z-1#3", "z-2#1"}
	if len(routes) != len(want) {
		t.Fatalf("got %d routes, want %d", len(routes), len(want))
	}
	for i, id := range want {
		if routes[i].ID != id {
			t.Errorf("routes[%d].ID = %q, want %q", i, routes[i].ID, id)
		}
	}
}
