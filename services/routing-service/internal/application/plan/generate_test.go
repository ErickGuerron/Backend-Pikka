package plan

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// trace records the order of side effects across fakes, so tests can prove that a
// driver is reserved before the orders of its route are assigned.
type trace struct {
	steps []string
}

func (t *trace) add(step string) {
	if t != nil {
		t.steps = append(t.steps, step)
	}
}

type fakeOrders struct {
	pending   []domain.PlannableOrder
	pendErr   error
	date      string
	assigned  []assignCall
	failAt    int // index of the AssignToRoute call that fails; -1 for none
	assignErr error
	trace     *trace
}

type assignCall struct {
	routeID string
	stops   []domain.RouteStop
}

func (f *fakeOrders) PendingByDate(_ context.Context, date string) ([]domain.PlannableOrder, error) {
	f.date = date
	return f.pending, f.pendErr
}

func (f *fakeOrders) AssignToRoute(_ context.Context, routeID string, stops []domain.RouteStop) error {
	if len(f.assigned) == f.failAt {
		return f.assignErr
	}
	f.assigned = append(f.assigned, assignCall{routeID: routeID, stops: stops})
	f.trace.add("assign " + routeID)
	return nil
}

type reservation struct {
	driverID string
	routeID  string
}

// fakeDrivers hands out the drivers in order, skipping the ones already reserved.
type fakeDrivers struct {
	available     []string
	findErr       error
	findCalls     int
	reserveErr    error
	reserveFailAt int // index of the Reserve call that fails; -1 for none
	reserved      []reservation
	released      []reservation
	releaseErr    error
	trace         *trace
}

func (f *fakeDrivers) FindAvailable(_ context.Context, n int) ([]string, error) {
	f.findCalls++
	if f.findErr != nil {
		return nil, f.findErr
	}
	var out []string
	for _, id := range f.available {
		if len(out) == n {
			break
		}
		if !f.isReserved(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeDrivers) Reserve(_ context.Context, driverID, routeID string) error {
	if len(f.reserved) == f.reserveFailAt {
		return f.reserveErr
	}
	f.reserved = append(f.reserved, reservation{driverID: driverID, routeID: routeID})
	f.trace.add("reserve " + driverID + " " + routeID)
	return nil
}

func (f *fakeDrivers) Release(_ context.Context, driverID, routeID string) error {
	f.released = append(f.released, reservation{driverID: driverID, routeID: routeID})
	f.trace.add("release " + driverID + " " + routeID)
	return f.releaseErr
}

func (f *fakeDrivers) isReserved(driverID string) bool {
	for _, r := range f.reserved {
		if r.driverID == driverID {
			return true
		}
	}
	return false
}

type fakeZones struct {
	capacity map[string]int
	err      error
	errZone  string // when set, only this zone fails
}

func (f fakeZones) MaxOrdersPerRoute(_ context.Context, zoneID string) (int, error) {
	if f.err != nil && (f.errZone == "" || f.errZone == zoneID) {
		return 0, f.err
	}
	return f.capacity[zoneID], nil
}

func newUseCase(orders *fakeOrders, zones fakeZones, drivers *fakeDrivers) *Generate {
	return New(orders, zones, drivers, domain.ZoneCapacityPlanner{})
}

func TestGenerateSplitsEachZoneByItsOwnCapacity(t *testing.T) {
	orders := &fakeOrders{
		failAt: -1,
		pending: []domain.PlannableOrder{
			{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
			{OrderID: "o4", ZoneID: "B"}, {OrderID: "o5", ZoneID: "B"},
		},
	}
	drivers := &fakeDrivers{available: []string{"d1", "d2", "d3"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 2, "B": 5}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
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
		{"2026-10-15-A-1", 2}, {"2026-10-15-A-2", 1}, {"2026-10-15-B-1", 2},
	}
	for i, want := range wantIDs {
		call := orders.assigned[i]
		if call.routeID != want.route || len(call.stops) != want.size {
			t.Fatalf("call %d = %s %+v, want %s with %d stops", i, call.routeID, call.stops, want.route, want.size)
		}
		for j, stop := range call.stops {
			if stop.Sequence != j+1 {
				t.Fatalf("call %d stop %d has sequence %d, want %d", i, j, stop.Sequence, j+1)
			}
		}
	}
	wantRoutes := []AssignedRoute{
		{RouteID: "2026-10-15-A-1", DriverID: "d1", OrderCount: 2},
		{RouteID: "2026-10-15-A-2", DriverID: "d2", OrderCount: 1},
		{RouteID: "2026-10-15-B-1", DriverID: "d3", OrderCount: 2},
	}
	if !slices.Equal(got.Routes, wantRoutes) {
		t.Fatalf("routes = %+v, want %+v", got.Routes, wantRoutes)
	}
}

func TestGenerateWithoutPendingOrdersDoesNothing(t *testing.T) {
	orders := &fakeOrders{failAt: -1}
	drivers := &fakeDrivers{reserveFailAt: -1}

	got, err := newUseCase(orders, fakeZones{}, drivers).Run(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Assigned) != 0 || len(orders.assigned) != 0 || drivers.findCalls != 0 {
		t.Fatalf("expected no routes, assignments or driver lookups, got %+v / %+v / %d", got, orders.assigned, drivers.findCalls)
	}
}

func TestGenerateRejectsInvalidDateBeforeGateway(t *testing.T) {
	orders := &fakeOrders{failAt: -1}
	drivers := &fakeDrivers{reserveFailAt: -1}

	_, err := newUseCase(orders, fakeZones{}, drivers).Run(context.Background(), "15/10/2026")
	if !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("err = %v, want ErrInvalidDate", err)
	}
	if orders.date != "" || drivers.findCalls != 0 {
		t.Fatalf("invalid date reached a gateway")
	}
}

func TestGenerateFailsBeforeReservingWhenZoneLookupFails(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{reserveFailAt: -1}
	zones := fakeZones{err: errors.New("zone service down")}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if err == nil {
		t.Fatal("expected error when capacity lookup fails")
	}
	if len(orders.assigned) != 0 || drivers.findCalls != 0 || len(drivers.reserved) != 0 {
		t.Fatalf("driver or assignment made despite planning error: %+v / %+v", orders.assigned, drivers.reserved)
	}
}

func TestGenerateReservesNothingWhenAnyZoneFailsToPlan(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "B"},
	}}
	drivers := &fakeDrivers{available: []string{"d1", "d2"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}, err: errors.New("zone B down"), errZone: "B"}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if err == nil {
		t.Fatal("expected error when zone B capacity lookup fails")
	}
	if drivers.findCalls != 0 || len(drivers.reserved) != 0 || len(orders.assigned) != 0 {
		t.Fatalf("zone A was reserved or assigned before zone B planning finished")
	}
}

func TestGenerateFailsBeforeAssigningOnZeroCapacity(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 0}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, domain.ErrInvalidCapacity) {
		t.Fatalf("err = %v, want ErrInvalidCapacity", err)
	}
	if len(orders.assigned) != 0 || len(drivers.reserved) != 0 {
		t.Fatalf("assignments made despite invalid capacity")
	}
}

func TestGenerateFailsBeforeAssigningOnInvalidOrder(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: ""}}}
	drivers := &fakeDrivers{reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("err = %v, want ErrInvalidOrder", err)
	}
	if len(orders.assigned) != 0 || len(drivers.reserved) != 0 {
		t.Fatalf("assignments made despite invalid order")
	}
}

func TestGenerateReservesDriverBeforeAssigningEachRoute(t *testing.T) {
	tr := &trace{}
	orders := &fakeOrders{failAt: -1, trace: tr, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
	}}
	drivers := &fakeDrivers{available: []string{"d1", "d2"}, reserveFailAt: -1, trace: tr}
	zones := fakeZones{capacity: map[string]int{"A": 2}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{
		"reserve d1 2026-10-15-A-1", "assign 2026-10-15-A-1",
		"reserve d2 2026-10-15-A-2", "assign 2026-10-15-A-2",
	}
	if !slices.Equal(tr.steps, want) {
		t.Fatalf("steps = %v, want %v", tr.steps, want)
	}
	wantRoutes := []AssignedRoute{
		{RouteID: "2026-10-15-A-1", DriverID: "d1", OrderCount: 2},
		{RouteID: "2026-10-15-A-2", DriverID: "d2", OrderCount: 1},
	}
	if !slices.Equal(got.Routes, wantRoutes) {
		t.Fatalf("routes = %+v, want %+v", got.Routes, wantRoutes)
	}
}

func TestGenerateStopsWhenNoDriverForLaterRoute(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
	}}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 2}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, ErrNoDriverAvailable) {
		t.Fatalf("err = %v, want ErrNoDriverAvailable", err)
	}
	if len(got.Routes) != 1 || got.Routes[0].RouteID != "2026-10-15-A-1" || got.Routes[0].DriverID != "d1" {
		t.Fatalf("routes before failure = %+v, want only A#1 with d1", got.Routes)
	}
	if len(orders.assigned) != 1 || orders.assigned[0].routeID != "2026-10-15-A-1" {
		t.Fatalf("assignments = %+v, want only A#1", orders.assigned)
	}
	for _, stop := range orders.assigned[0].stops {
		if stop.OrderID == "o3" {
			t.Fatalf("order o3 of the failing route was assigned")
		}
	}
}

func TestGenerateTouchesNoOrdersWhenNoDriverAtAll(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, ErrNoDriverAvailable) {
		t.Fatalf("err = %v, want ErrNoDriverAvailable", err)
	}
	if len(got.Routes) != 0 || len(orders.assigned) != 0 || len(drivers.reserved) != 0 {
		t.Fatalf("orders or drivers touched with no driver: %+v / %+v", orders.assigned, drivers.reserved)
	}
}

func TestGenerateStopsWhenReserveFails(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"}, {OrderID: "o3", ZoneID: "A"},
	}}
	reserveErr := errors.New("driver already reserved")
	drivers := &fakeDrivers{available: []string{"d1", "d2"}, reserveFailAt: 1, reserveErr: reserveErr}
	zones := fakeZones{capacity: map[string]int{"A": 2}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, reserveErr) {
		t.Fatalf("err = %v, want the reserve error", err)
	}
	if len(got.Routes) != 1 || got.Routes[0].RouteID != "2026-10-15-A-1" {
		t.Fatalf("routes before failure = %+v, want only A#1", got.Routes)
	}
	if len(orders.assigned) != 1 {
		t.Fatalf("assignments after failed reserve: %+v", orders.assigned)
	}
}

func TestGenerateStopsWhenDriverLookupFails(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	lookupErr := errors.New("driver service down")
	drivers := &fakeDrivers{findErr: lookupErr, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("err = %v, want the lookup error", err)
	}
	if len(orders.assigned) != 0 {
		t.Fatalf("orders assigned despite driver lookup failure")
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
	drivers := &fakeDrivers{available: []string{"d1", "d2", "d3"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 2}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")
	if err == nil {
		t.Fatal("expected error when a route assignment fails")
	}
	if len(got.Assigned) != 1 || got.Assigned[0].ID != "A#1" {
		t.Fatalf("assigned before failure = %+v, want only A#1", got.Assigned)
	}
	if len(got.Routes) != 1 || got.Routes[0].DriverID != "d1" {
		t.Fatalf("routes before failure = %+v, want only A#1 with d1", got.Routes)
	}
	if len(orders.assigned) != 1 {
		t.Fatalf("assignments after failure: %+v", orders.assigned)
	}
	// The failing route's driver is released, so it is not left reserved for a route with no orders.
	want := []reservation{{driverID: "d2", routeID: "2026-10-15-A-2"}}
	if !slices.Equal(drivers.released, want) {
		t.Fatalf("released = %+v, want %+v", drivers.released, want)
	}
}

func TestGenerateReleasesDriverWhenAssignFails(t *testing.T) {
	assignErr := errors.New("order gateway unavailable")
	orders := &fakeOrders{failAt: 0, assignErr: assignErr, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"},
	}}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	got, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")

	if !errors.Is(err, assignErr) {
		t.Fatalf("err = %v, want it to wrap the assign error", err)
	}
	want := []reservation{{driverID: "d1", routeID: "2026-10-15-A-1"}}
	if !slices.Equal(drivers.released, want) {
		t.Fatalf("released = %+v, want %+v", drivers.released, want)
	}
	if len(got.Routes) != 0 {
		t.Fatalf("routes = %+v, want none: the failing route is not reported", got.Routes)
	}
}

func TestGenerateReleasesBeforeStoppingAfterAssignFails(t *testing.T) {
	tr := &trace{}
	assignErr := errors.New("order gateway unavailable")
	orders := &fakeOrders{failAt: 1, assignErr: assignErr, trace: tr, pending: []domain.PlannableOrder{
		{OrderID: "o1", ZoneID: "A"}, {OrderID: "o2", ZoneID: "A"},
	}}
	drivers := &fakeDrivers{available: []string{"d1", "d2"}, reserveFailAt: -1, trace: tr}
	zones := fakeZones{capacity: map[string]int{"A": 1}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")

	if !errors.Is(err, assignErr) {
		t.Fatalf("err = %v, want the assign error", err)
	}
	want := []string{
		"reserve d1 2026-10-15-A-1", "assign 2026-10-15-A-1",
		"reserve d2 2026-10-15-A-2", "release d2 2026-10-15-A-2",
	}
	if !slices.Equal(tr.steps, want) {
		t.Fatalf("steps = %v, want %v", tr.steps, want)
	}
}

func TestGenerateReportsReleaseFailureWithAssignError(t *testing.T) {
	assignErr := errors.New("order gateway unavailable")
	releaseErr := errors.New("driver service unavailable")
	orders := &fakeOrders{failAt: 0, assignErr: assignErr, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: -1, releaseErr: releaseErr}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")

	if !errors.Is(err, assignErr) {
		t.Fatalf("err = %v, want it to keep the assign error", err)
	}
	if !errors.Is(err, releaseErr) {
		t.Fatalf("err = %v, want it to report the release error too", err)
	}
	if len(drivers.released) != 1 {
		t.Fatalf("release attempts = %d, want 1", len(drivers.released))
	}
}

func TestGenerateDoesNotReleaseWhenReserveFails(t *testing.T) {
	orders := &fakeOrders{failAt: -1, pending: []domain.PlannableOrder{{OrderID: "o1", ZoneID: "A"}}}
	reserveErr := errors.New("driver already reserved")
	drivers := &fakeDrivers{available: []string{"d1"}, reserveFailAt: 0, reserveErr: reserveErr}
	zones := fakeZones{capacity: map[string]int{"A": 5}}

	_, err := newUseCase(orders, zones, drivers).Run(context.Background(), "2026-10-15")

	if !errors.Is(err, reserveErr) {
		t.Fatalf("err = %v, want the reserve error", err)
	}
	if len(drivers.released) != 0 {
		t.Fatalf("released %+v without a reservation", drivers.released)
	}
}
