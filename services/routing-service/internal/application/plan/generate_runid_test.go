package plan

import (
	"context"
	"regexp"
	"slices"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// Planning the same date and zone twice used to reuse "{date}-{zone}-1", the id of a route that
// already existed, and Order Service rejected it with "stop sequences must be unique within a route".
func TestGenerateTwiceSameDateAndZoneUsesNewRouteIDs(t *testing.T) {
	tokens := []string{"aaa111", "bbb222"}
	next := func() string { tok := tokens[0]; tokens = tokens[1:]; return tok }
	drivers := &fakeDrivers{available: []string{"d1", "d2"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 4}}
	uc := newUseCase(&fakeOrders{failAt: -1}, zones, drivers).WithRunIDs(next)

	first := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	uc.orders = first
	got1, err := uc.Run(context.Background(), "2026-11-19")
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o2", ZoneID: "A"}}}
	uc.orders = second
	got2, err := uc.Run(context.Background(), "2026-11-19")
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	id1, id2 := got1.Routes[0].RouteID, got2.Routes[0].RouteID
	if id1 != "2026-11-19-A-1-aaa111" || id2 != "2026-11-19-A-1-bbb222" {
		t.Fatalf("route ids = %q, %q; want 2026-11-19-A-1-aaa111 and 2026-11-19-A-1-bbb222", id1, id2)
	}
	if second.assigned[0].routeID == first.assigned[0].routeID {
		t.Fatalf("second run reused route id %q", first.assigned[0].routeID)
	}
}

// Every route of one run shares the run token, so its routes are still numbered 1..n.
func TestGenerateUsesOneTokenPerRun(t *testing.T) {
	calls := 0
	next := func() string { calls++; return "run777" }
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
	}}
	drivers := &fakeDrivers{available: []string{"d1", "d2"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 2}}

	got, err := newUseCase(orders, zones, drivers).WithRunIDs(next).Run(context.Background(), "2026-11-19")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ids := []string{got.Routes[0].RouteID, got.Routes[1].RouteID}
	if !slices.Equal(ids, []string{"2026-11-19-A-1-run777", "2026-11-19-A-2-run777"}) || calls != 1 {
		t.Fatalf("ids = %v (token asked %d times), want one token for the whole run", ids, calls)
	}
}

// The route id travels in the driver stop URL, so the token must keep it path safe.
func TestRandomRunIDIsShortHexAndVaries(t *testing.T) {
	hexRe := regexp.MustCompile(`^[0-9a-f]{6}$`)
	seen := map[string]bool{}
	for range 50 {
		id := RandomRunID()
		if !hexRe.MatchString(id) {
			t.Fatalf("RandomRunID() = %q, want 6 hex characters", id)
		}
		seen[id] = true
	}
	if len(seen) < 45 {
		t.Fatalf("RandomRunID repeated too often: %d distinct of 50", len(seen))
	}
}
