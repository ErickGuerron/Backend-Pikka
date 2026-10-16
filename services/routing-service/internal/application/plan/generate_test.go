package plan

import (
	"context"
	"errors"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

type fakeOrders struct {
	pending   []domain.PlannableOrder
	pendErr   error
	date      string
	assigned  []assignCall
	failAt    int // index of the AssignToRoute call that fails; -1 for none
	assignErr error
}

type assignCall struct {
	routeID  string
	orderIDs []string
}

func (f *fakeOrders) PendingByDate(_ context.Context, date string) ([]domain.PlannableOrder, error) {
	f.date = date
	return f.pending, f.pendErr
}

func (f *fakeOrders) AssignToRoute(_ context.Context, routeID string, orderIDs []string) error {
	if len(f.assigned) == f.failAt {
		return f.assignErr
	}
	f.assigned = append(f.assigned, assignCall{routeID: routeID, orderIDs: orderIDs})
	return nil
}

type fakeZones struct {
	capacity map[string]int
	err      error
}

func (f fakeZones) MaxOrdersPerRoute(_ context.Context, zoneID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.capacity[zoneID], nil
}

func newUseCase(orders *fakeOrders, zones fakeZones) *Generate {
	return New(orders, zones, domain.ZoneCapacityPlanner{})
}

func TestGenerateSplitsEachZoneByItsOwnCapacity(t *testing.T) {
	orders := &fakeOrders{
		failAt: -1,
		pending: []domain.PlannableOrder{
			{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
			{OrderID: "o4", ZoneID: "B"}, {OrderID: "o5", ZoneID: "B"},
		},
	}
	zones := fakeZones{capacity: map[string]int{"A": 2, "B": 5}}

	got, err := newUseCase(orders, zones).Run(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Assigned) != 3 {
		t.Fatalf("assigned routes = %d, want 3 (A: 2+1, B: 1)", len(got.Assigned))
	}
	if orders.date != "2026-10-15" {
		t.Fatalf("pending date = %q", orders.date)
	}
	wantIDs := []struct {
		route string
		size  int
	}{
		{"2026-10-15/A#1", 2}, {"2026-10-15/A#2", 1}, {"2026-10-15/B#1", 2},
	}
	for i, want := range wantIDs {
		call := orders.assigned[i]
		if call.routeID != want.route || len(call.orderIDs) != want.size {
			t.Fatalf("call %d = %s %v, want %s with %d orders", i, call.routeID, call.orderIDs, want.route, want.size)
		}
	}
}

func TestGenerateWithoutPendingOrdersDoesNothing(t *testing.T) {
	orders := &fakeOrders{failAt: -1}

	got, err := newUseCase(orders, fakeZones{}).Run(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Assigned) != 0 || len(orders.assigned) != 0 {
		t.Fatalf("expected no routes and no assignments, got %+v / %+v", got, orders.assigned)
	}
}

func TestGenerateRejectsInvalidDateBeforeGateway(t *testing.T) {
	orders := &fakeOrders{failAt: -1}

	_, err := newUseCase(orders, fakeZones{}).Run(context.Background(), "15/10/2026")
	if !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("err = %v, want ErrInvalidDate", err)
	}
	if orders.date != "" {
		t.Fatalf("invalid date reached the order gateway")
	}
}

func TestGenerateFailsBeforeAssigningWhenZoneLookupFails(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	zones := fakeZones{err: errors.New("zone service down")}

	_, err := newUseCase(orders, zones).Run(context.Background(), "2026-10-15")
	if err == nil {
		t.Fatal("expected error when capacity lookup fails")
	}
	if len(orders.assigned) != 0 {
		t.Fatalf("assignments made despite planning error: %+v", orders.assigned)
	}
}

func TestGenerateFailsBeforeAssigningOnZeroCapacity(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	zones := fakeZones{capacity: map[string]int{"A": 0}}

	_, err := newUseCase(orders, zones).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, domain.ErrInvalidCapacity) {
		t.Fatalf("err = %v, want ErrInvalidCapacity", err)
	}
	if len(orders.assigned) != 0 {
		t.Fatalf("assignments made despite invalid capacity")
	}
}

func TestGenerateFailsBeforeAssigningOnInvalidOrder(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: ""}}}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	_, err := newUseCase(orders, zones).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("err = %v, want ErrInvalidOrder", err)
	}
	if len(orders.assigned) != 0 {
		t.Fatalf("assignments made despite invalid order")
	}
}

func TestGenerateReportsRoutesAssignedBeforeAFailure(t *testing.T) {
	orders := &fakeOrders{
		failAt:    1,
		assignErr: errors.New("order gateway unavailable"),
		pending: []domain.PlannableOrder{
			{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
		},
	}
	zones := fakeZones{capacity: map[string]int{"A": 2}}

	got, err := newUseCase(orders, zones).Run(context.Background(), "2026-10-15")
	if err == nil {
		t.Fatal("expected error when a route assignment fails")
	}
	if len(got.Assigned) != 1 || got.Assigned[0].ID != "A#1" {
		t.Fatalf("assigned before failure = %+v, want only A#1", got.Assigned)
	}
	if len(orders.assigned) != 1 {
		t.Fatalf("assignments after failure: %+v", orders.assigned)
	}
}
