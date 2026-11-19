package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// outcomeRepo embeds the port so only AdvanceRouteStop, the method the outcome use case calls, is
// implemented. It counts calls so tests can prove invalid input never reaches the repository.
type outcomeRepo struct {
	ports.OrderRepository
	calls   int
	current domain.Order
	routeID string
	orderID string
}

func (r *outcomeRepo) AdvanceRouteStop(_ context.Context, orderID, routeID string, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	r.calls++
	r.orderID, r.routeID = orderID, routeID
	return decide(r.current)
}

func TestRecordStopOutcomeClosesInTransitStop(t *testing.T) {
	for _, outcome := range []domain.Status{domain.StatusDelivered, domain.StatusNotDelivered} {
		id := uuid.NewString()
		repo := &outcomeRepo{current: domain.Order{ID: id, Status: domain.StatusInTransit, RouteID: "zone-1#1"}}

		got, err := NewRecordStopOutcome(repo).Run(context.Background(), RecordStopOutcomeCommand{
			Role: RoleDriver, RouteID: "zone-1#1", OrderID: id, Outcome: outcome,
		})
		if err != nil {
			t.Fatalf("%s: Run: %v", outcome, err)
		}
		if got.Status != outcome {
			t.Fatalf("%s: status = %s", outcome, got.Status)
		}
		if repo.routeID != "zone-1#1" || repo.orderID != id {
			t.Fatalf("%s: repo got route %q order %q", outcome, repo.routeID, repo.orderID)
		}
	}
}

func TestRecordStopOutcomeRejectsInvalidInputBeforeRepository(t *testing.T) {
	validID := uuid.NewString()
	cases := []struct {
		name string
		cmd  RecordStopOutcomeCommand
		want error
	}{
		{"not driver", RecordStopOutcomeCommand{Role: RoleOperator, RouteID: "r", OrderID: validID, Outcome: domain.StatusDelivered}, domain.ErrNotDriver},
		{"blank route", RecordStopOutcomeCommand{Role: RoleDriver, RouteID: "  ", OrderID: validID, Outcome: domain.StatusDelivered}, domain.ErrInvalidRouteID},
		{"bad order id", RecordStopOutcomeCommand{Role: RoleDriver, RouteID: "r", OrderID: "nope", Outcome: domain.StatusDelivered}, domain.ErrInvalidOrderID},
		{"picked up is not an outcome", RecordStopOutcomeCommand{Role: RoleDriver, RouteID: "r", OrderID: validID, Outcome: domain.StatusPickedUp}, domain.ErrInvalidStopOutcome},
		{"empty outcome", RecordStopOutcomeCommand{Role: RoleDriver, RouteID: "r", OrderID: validID, Outcome: ""}, domain.ErrInvalidStopOutcome},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &outcomeRepo{}
			_, err := NewRecordStopOutcome(repo).Run(context.Background(), tc.cmd)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != 0 {
				t.Fatalf("repository called %d times on invalid input", repo.calls)
			}
		})
	}
}

func TestRecordStopOutcomeRejectsStopNotInTransit(t *testing.T) {
	id := uuid.NewString()
	repo := &outcomeRepo{current: domain.Order{ID: id, Status: domain.StatusRouteAssigned, RouteID: "r"}}

	_, err := NewRecordStopOutcome(repo).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "r", OrderID: id, Outcome: domain.StatusDelivered,
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}
