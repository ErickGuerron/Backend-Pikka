package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// stopRouteRepo calls decide for every id, as the real repository does under its lock, and records
// the decided order per id so tests can read the stop sequence each order received.
type stopRouteRepo struct {
	ids     []string
	decided map[string]domain.Order
	calls   int
}

func (r *stopRouteRepo) ListPendingByDate(_ context.Context, _ string) ([]domain.Order, error) {
	return nil, nil
}

func (r *stopRouteRepo) AssignToRoute(_ context.Context, ids []string, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	r.calls++
	r.ids = ids
	r.decided = map[string]domain.Order{}
	out := make([]domain.Order, 0, len(ids))
	for _, id := range ids {
		next, err := decide(domain.Order{ID: id, Status: domain.StatusPending})
		if err != nil {
			return nil, err
		}
		r.decided[id] = next
		out = append(out, next)
	}
	return out, nil
}

func TestAssignWithStopsPersistsEachSequence(t *testing.T) {
	first, second := uuid.NewString(), uuid.NewString()
	repo := &stopRouteRepo{}

	got, err := NewRouteAssignment(repo).Assign(context.Background(), AssignToRouteCommand{
		Role:    RoleOperator,
		RouteID: "zone-1#1",
		Stops: []RouteStop{
			{OrderID: second, Sequence: 2},
			{OrderID: first, Sequence: 1},
		},
	})

	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(repo.ids) != 2 || repo.ids[0] != second || repo.ids[1] != first {
		t.Fatalf("ids = %v, want the stop order as received", repo.ids)
	}
	if repo.decided[second].StopSequence != 2 || repo.decided[first].StopSequence != 1 {
		t.Fatalf("sequences = %d and %d, want 2 and 1", repo.decided[second].StopSequence, repo.decided[first].StopSequence)
	}
	for _, o := range got {
		if o.Status != domain.StatusRouteAssigned || o.RouteID != "zone-1#1" {
			t.Fatalf("order %s = %s on %q, want ROUTE_ASSIGNED on zone-1#1", o.ID, o.Status, o.RouteID)
		}
	}
}

// When both forms are sent, stops wins and order_ids is ignored.
func TestAssignStopsWinOverOrderIDs(t *testing.T) {
	stopID, legacyID := uuid.NewString(), uuid.NewString()
	repo := &stopRouteRepo{}

	_, err := NewRouteAssignment(repo).Assign(context.Background(), AssignToRouteCommand{
		Role:     RoleOperator,
		RouteID:  "zone-1#1",
		OrderIDs: []string{legacyID},
		Stops:    []RouteStop{{OrderID: stopID, Sequence: 1}},
	})

	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(repo.ids) != 1 || repo.ids[0] != stopID {
		t.Fatalf("ids = %v, want only the stop order", repo.ids)
	}
	if repo.decided[stopID].StopSequence != 1 {
		t.Fatalf("sequence = %d, want 1", repo.decided[stopID].StopSequence)
	}
}

// The legacy order_ids form has no sequence, so the order stays unassigned (0).
func TestAssignLegacyOrderIDsLeavesSequenceUnassigned(t *testing.T) {
	id := uuid.NewString()
	repo := &stopRouteRepo{}

	_, err := NewRouteAssignment(repo).Assign(context.Background(), AssignToRouteCommand{
		Role:     RoleOperator,
		RouteID:  "zone-1#1",
		OrderIDs: []string{id},
	})

	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if repo.decided[id].StopSequence != 0 {
		t.Fatalf("sequence = %d, want 0 (unassigned)", repo.decided[id].StopSequence)
	}
}

func TestAssignStopsRejectInvalidSequenceBeforeRepository(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	cases := []struct {
		name  string
		stops []RouteStop
		want  error
	}{
		{"zero sequence", []RouteStop{{OrderID: a, Sequence: 0}}, domain.ErrInvalidStopSequence},
		{"negative sequence", []RouteStop{{OrderID: a, Sequence: -3}}, domain.ErrInvalidStopSequence},
		{"duplicate sequence", []RouteStop{{OrderID: a, Sequence: 2}, {OrderID: b, Sequence: 2}}, domain.ErrDuplicateStopSequence},
		{"duplicate order", []RouteStop{{OrderID: a, Sequence: 1}, {OrderID: a, Sequence: 2}}, domain.ErrDuplicateOrderID},
		{"not a uuid", []RouteStop{{OrderID: "abc", Sequence: 1}}, domain.ErrInvalidOrderID},
	}
	for _, tc := range cases {
		repo := &stopRouteRepo{}

		_, err := NewRouteAssignment(repo).Assign(context.Background(), AssignToRouteCommand{
			Role: RoleOperator, RouteID: "zone-1#1", Stops: tc.stops,
		})

		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if repo.calls != 0 {
			t.Fatalf("%s: reached the repository", tc.name)
		}
	}
}

// advanceRepo applies the same rule as the SQL query (WHERE id AND route_id) and records calls.
type advanceRepo struct {
	ports.OrderRepository // not implemented here: only AdvanceRouteStop is called
	orders                map[string]domain.Order
	calls                 int
}

func (r *advanceRepo) AdvanceRouteStop(_ context.Context, orderID, routeID string, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	r.calls++
	order, ok := r.orders[orderID]
	if !ok || order.RouteID != routeID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	next, err := decide(order)
	if err != nil {
		return domain.Order{}, err
	}
	r.orders[orderID] = next
	return next, nil
}

const driverRoute = "zone-1#1"

func advanceFixture(t *testing.T, status domain.Status) (*AdvanceRouteStop, *advanceRepo, string) {
	t.Helper()
	id := uuid.New().String()
	repo := &advanceRepo{orders: map[string]domain.Order{
		id: {ID: id, RouteID: driverRoute, Status: status, StopSequence: 1},
	}}
	return NewAdvanceRouteStop(repo), repo, id
}

func TestAdvanceRouteStopMovesOwnRouteStepByStep(t *testing.T) {
	uc, repo, id := advanceFixture(t, domain.StatusRouteAssigned)

	picked, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: driverRoute, OrderID: id, Target: domain.StatusPickedUp})
	if err != nil {
		t.Fatalf("to PICKED_UP: %v", err)
	}
	if picked.Status != domain.StatusPickedUp {
		t.Fatalf("status = %s, want PICKED_UP", picked.Status)
	}

	transit, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: driverRoute, OrderID: id, Target: domain.StatusInTransit})
	if err != nil {
		t.Fatalf("to IN_TRANSIT: %v", err)
	}
	if transit.Status != domain.StatusInTransit || transit.StopSequence != 1 || transit.RouteID != driverRoute {
		t.Fatalf("order = %+v, want IN_TRANSIT keeping route and sequence", transit)
	}
	if repo.orders[id].Status != domain.StatusInTransit {
		t.Fatalf("stored status = %s, want IN_TRANSIT", repo.orders[id].Status)
	}
}

func TestAdvanceRouteStopOrderOnAnotherRouteIsNotFound(t *testing.T) {
	uc, repo, id := advanceFixture(t, domain.StatusRouteAssigned)

	_, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: "zone-9#4", OrderID: id, Target: domain.StatusPickedUp})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
	if repo.orders[id].Status != domain.StatusRouteAssigned {
		t.Fatalf("order changed to %s on a wrong route", repo.orders[id].Status)
	}
}

func TestAdvanceRouteStopUnknownOrderIsNotFound(t *testing.T) {
	uc, _, _ := advanceFixture(t, domain.StatusRouteAssigned)

	_, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: driverRoute, OrderID: uuid.NewString(), Target: domain.StatusPickedUp})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
}

func TestAdvanceRouteStopIllegalTransitionIsRejected(t *testing.T) {
	cases := []struct {
		name   string
		status domain.Status
		target domain.Status
	}{
		{"skip PICKED_UP", domain.StatusRouteAssigned, domain.StatusInTransit},
		{"repeat PICKED_UP", domain.StatusPickedUp, domain.StatusPickedUp},
		{"go back", domain.StatusInTransit, domain.StatusPickedUp},
		{"from PENDING", domain.StatusPending, domain.StatusPickedUp},
	}
	for _, tc := range cases {
		uc, repo, id := advanceFixture(t, tc.status)

		_, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: driverRoute, OrderID: id, Target: tc.target})

		if !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidTransition", tc.name, err)
		}
		if repo.orders[id].Status != tc.status {
			t.Fatalf("%s: status changed to %s on rejection", tc.name, repo.orders[id].Status)
		}
	}
}

func TestAdvanceRouteStopRejectsTargetBeforeRepository(t *testing.T) {
	for _, target := range []domain.Status{domain.StatusDelivered, domain.StatusNotDelivered, domain.Status("")} {
		uc, repo, id := advanceFixture(t, domain.StatusInTransit)

		_, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: driverRoute, OrderID: id, Target: target})

		if !errors.Is(err, domain.ErrInvalidAdvanceTarget) {
			t.Fatalf("target %q: err = %v, want ErrInvalidAdvanceTarget", target, err)
		}
		if repo.calls != 0 {
			t.Fatalf("target %q reached the repository", target)
		}
	}
}

func TestAdvanceRouteStopRejectsNonDriverBeforeRepository(t *testing.T) {
	for _, role := range []string{"", RoleCustomer, RoleOperator, RoleAdmin} {
		uc, repo, id := advanceFixture(t, domain.StatusRouteAssigned)

		_, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: role, RouteID: driverRoute, OrderID: id, Target: domain.StatusPickedUp})

		if !errors.Is(err, domain.ErrNotDriver) {
			t.Fatalf("role %q: err = %v, want ErrNotDriver", role, err)
		}
		if repo.calls != 0 {
			t.Fatalf("role %q reached the repository", role)
		}
	}
}

func TestAdvanceRouteStopRejectsBlankIdentifiersBeforeRepository(t *testing.T) {
	validID := uuid.NewString()
	cases := []struct {
		name    string
		routeID string
		orderID string
		want    error
	}{
		{"blank route", "  ", validID, domain.ErrInvalidRouteID},
		{"blank order", driverRoute, "", domain.ErrInvalidOrderID},
		{"order not a uuid", driverRoute, "abc", domain.ErrInvalidOrderID},
	}
	for _, tc := range cases {
		uc, repo, _ := advanceFixture(t, domain.StatusRouteAssigned)

		_, err := uc.Run(context.Background(), AdvanceRouteStopCommand{Role: RoleDriver, RouteID: tc.routeID, OrderID: tc.orderID, Target: domain.StatusPickedUp})

		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if repo.calls != 0 {
			t.Fatalf("%s reached the repository", tc.name)
		}
	}
}
