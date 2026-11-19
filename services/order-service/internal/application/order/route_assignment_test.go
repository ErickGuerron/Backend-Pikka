package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type routeRepo struct {
	pendingDate string
	assignIDs   []string
	decided     int
	result      []domain.Order
	err         error
}

func (r *routeRepo) ListPendingByDate(_ context.Context, date string) ([]domain.Order, error) {
	r.pendingDate = date
	return r.result, r.err
}

func (r *routeRepo) AssignToRoute(_ context.Context, ids []string, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	r.assignIDs = ids
	if _, err := decide(domain.Order{Status: domain.StatusPending}); err == nil {
		r.decided++
	}
	return r.result, r.err
}

func TestRouteRejectsNonOperatorsBeforeRepository(t *testing.T) {
	valid := uuid.NewString()
	for _, role := range []string{"", RoleCustomer, "DRIVER"} {
		repo := &routeRepo{}
		uc := NewRouteAssignment(repo)

		if _, err := uc.PendingByDate(context.Background(), role, "2026-10-15"); !errors.Is(err, domain.ErrNotOperator) {
			t.Fatalf("PendingByDate role %q: err = %v, want ErrNotOperator", role, err)
		}
		if _, err := uc.Assign(context.Background(), AssignToRouteCommand{Role: role, RouteID: "r1", OrderIDs: []string{valid}}); !errors.Is(err, domain.ErrNotOperator) {
			t.Fatalf("Assign role %q: err = %v, want ErrNotOperator", role, err)
		}
		if repo.pendingDate != "" || repo.assignIDs != nil {
			t.Fatalf("role %q reached the repository", role)
		}
	}
}

func TestRouteAllowsAdminAndOperator(t *testing.T) {
	for _, role := range []string{RoleAdmin, RoleOperator} {
		repo := &routeRepo{}
		if _, err := NewRouteAssignment(repo).PendingByDate(context.Background(), role, "2026-10-15"); err != nil {
			t.Fatalf("role %s: PendingByDate: %v", role, err)
		}
	}
}

func TestPendingByDateRejectsInvalidDateBeforeQuery(t *testing.T) {
	for _, date := range []string{"", "2026-13-01", "2026-02-30", "15/10/2026"} {
		repo := &routeRepo{}
		_, err := NewRouteAssignment(repo).PendingByDate(context.Background(), RoleOperator, date)
		if !errors.Is(err, domain.ErrInvalidDate) {
			t.Fatalf("date %q: err = %v, want ErrInvalidDate", date, err)
		}
		if repo.pendingDate != "" {
			t.Fatalf("date %q reached the repository", date)
		}
	}
}

func TestPendingByDatePassesValidDate(t *testing.T) {
	repo := &routeRepo{}
	if _, err := NewRouteAssignment(repo).PendingByDate(context.Background(), RoleOperator, "2026-10-15"); err != nil {
		t.Fatalf("PendingByDate: %v", err)
	}
	if repo.pendingDate != "2026-10-15" {
		t.Fatalf("repository date = %q", repo.pendingDate)
	}
}

func TestAssignRejectsInvalidInputBeforeRepository(t *testing.T) {
	valid := uuid.NewString()
	cases := []struct {
		name string
		cmd  AssignToRouteCommand
		want error
	}{
		{"blank route", AssignToRouteCommand{Role: RoleOperator, RouteID: "  ", OrderIDs: []string{valid}}, domain.ErrInvalidRouteID},
		{"no orders", AssignToRouteCommand{Role: RoleOperator, RouteID: "r1"}, domain.ErrNoOrdersToAssign},
		{"not a uuid", AssignToRouteCommand{Role: RoleOperator, RouteID: "r1", OrderIDs: []string{"abc"}}, domain.ErrInvalidOrderID},
		{"duplicate", AssignToRouteCommand{Role: RoleOperator, RouteID: "r1", OrderIDs: []string{valid, valid}}, domain.ErrDuplicateOrderID},
	}
	for _, tc := range cases {
		repo := &routeRepo{}
		_, err := NewRouteAssignment(repo).Assign(context.Background(), tc.cmd)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if repo.assignIDs != nil {
			t.Fatalf("%s: reached the repository", tc.name)
		}
	}
}

func TestAssignPassesNormalizedIDsAndDecision(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	repo := &routeRepo{result: []domain.Order{{Status: domain.StatusRouteAssigned}}}

	got, err := NewRouteAssignment(repo).Assign(context.Background(), AssignToRouteCommand{
		Role:     RoleOperator,
		RouteID:  "zone-1#1",
		OrderIDs: []string{first.String(), second.String()},
	})
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(got) != 1 || got[0].Status != domain.StatusRouteAssigned {
		t.Fatalf("result = %+v", got)
	}
	if len(repo.assignIDs) != 2 || repo.assignIDs[0] != first.String() || repo.assignIDs[1] != second.String() {
		t.Fatalf("ids = %v", repo.assignIDs)
	}
	if repo.decided != 1 {
		t.Fatalf("decision applied %d times, want 1", repo.decided)
	}
}
