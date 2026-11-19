package order

import (
	"context"
	"errors"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type listAllRepo struct {
	ports.OrderRepository
	calls  int
	filter ports.OrderFilter
	result []domain.Order
	err    error
}

func (r *listAllRepo) ListAll(_ context.Context, f ports.OrderFilter) ([]domain.Order, error) {
	r.calls++
	r.filter = f
	return r.result, r.err
}

func TestListOrdersRejectsNonOperatorsBeforeTheRepository(t *testing.T) {
	for _, role := range []string{"", RoleCustomer, "DRIVER"} {
		repo := &listAllRepo{}

		_, err := NewListOrders(repo).Run(context.Background(), ListOrdersQuery{Role: role})

		if !errors.Is(err, domain.ErrNotOperator) {
			t.Fatalf("role %q: err = %v, want ErrNotOperator", role, err)
		}
		if repo.calls != 0 {
			t.Fatalf("role %q reached the repository", role)
		}
	}
}

func TestListOrdersPassesTheFilterAndReturnsTheOrders(t *testing.T) {
	for _, role := range []string{RoleAdmin, RoleOperator} {
		want := []domain.Order{{ID: "o-1"}, {ID: "o-2"}}
		repo := &listAllRepo{result: want}

		got, err := NewListOrders(repo).Run(context.Background(), ListOrdersQuery{
			Role: role, DeliveryDate: "2026-10-20", Status: domain.StatusPendingReview,
		})

		if err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
		if len(got) != 2 || got[0].ID != "o-1" {
			t.Fatalf("role %s: got %+v", role, got)
		}
		if repo.filter.DeliveryDate != "2026-10-20" || repo.filter.Status != domain.StatusPendingReview {
			t.Fatalf("role %s: filter = %+v", role, repo.filter)
		}
	}
}

func TestListOrdersWithoutFilterListsEverything(t *testing.T) {
	repo := &listAllRepo{}

	if _, err := NewListOrders(repo).Run(context.Background(), ListOrdersQuery{Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if repo.filter != (ports.OrderFilter{}) {
		t.Fatalf("filter = %+v, want empty", repo.filter)
	}
}

func TestListOrdersRejectsAMalformedDateBeforeTheRepository(t *testing.T) {
	for _, date := range []string{"15/10/2026", "2026-13-45", "mañana", "2026-10-20T00:00:00Z"} {
		repo := &listAllRepo{}

		_, err := NewListOrders(repo).Run(context.Background(), ListOrdersQuery{Role: RoleAdmin, DeliveryDate: date})

		if !errors.Is(err, ErrInvalidDeliveryDate) {
			t.Fatalf("date %q: err = %v, want ErrInvalidDeliveryDate", date, err)
		}
		if repo.calls != 0 {
			t.Fatalf("date %q reached the repository", date)
		}
	}
}

func TestListOrdersPropagatesRepositoryErrors(t *testing.T) {
	boom := errors.New("db down")

	_, err := NewListOrders(&listAllRepo{err: boom}).Run(context.Background(), ListOrdersQuery{Role: RoleAdmin})

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the repository error", err)
	}
}
