package order

import (
	"context"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// driverRouteRepo registra la ruta pedida y devuelve los pedidos preparados.
type driverRouteRepo struct {
	orders   []domain.Order
	err      error
	calls    int
	gotRoute string
}

func (r *driverRouteRepo) ListDriverRouteStops(_ context.Context, routeID string) ([]domain.Order, error) {
	r.calls++
	r.gotRoute = routeID
	return r.orders, r.err
}

func TestDriverRouteStopsDriverGetsOrderedStops(t *testing.T) {
	repo := &driverRouteRepo{orders: []domain.Order{
		{ID: "o-1", RouteID: "route-7", StopSequence: 1},
		{ID: "o-2", RouteID: "route-7", StopSequence: 2},
	}}
	uc := NewDriverRouteStops(repo)

	got, err := uc.Run(context.Background(), DriverRouteStopsCommand{Role: RoleDriver, RouteID: "  route-7  "})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 2 || got[0].ID != "o-1" || got[1].ID != "o-2" {
		t.Fatalf("stops = %+v, want o-1 then o-2", got)
	}
	if repo.gotRoute != "route-7" {
		t.Fatalf("repository route = %q, want the trimmed id route-7", repo.gotRoute)
	}
}

func TestDriverRouteStopsRejectsNonDriverBeforeReading(t *testing.T) {
	for _, role := range []string{RoleCustomer, "ADMIN", "OPERATOR", ""} {
		repo := &driverRouteRepo{}
		uc := NewDriverRouteStops(repo)

		_, err := uc.Run(context.Background(), DriverRouteStopsCommand{Role: role, RouteID: "route-7"})

		if err != domain.ErrNotDriver {
			t.Fatalf("role %q: err = %v, want ErrNotDriver", role, err)
		}
		if repo.calls != 0 {
			t.Fatalf("role %q: repository was called %d times, want 0", role, repo.calls)
		}
	}
}

func TestDriverRouteStopsBlankRouteIsInvalid(t *testing.T) {
	repo := &driverRouteRepo{}
	uc := NewDriverRouteStops(repo)

	_, err := uc.Run(context.Background(), DriverRouteStopsCommand{Role: RoleDriver, RouteID: "   "})

	if err != domain.ErrInvalidRouteID {
		t.Fatalf("err = %v, want ErrInvalidRouteID", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository was called %d times, want 0", repo.calls)
	}
}

func TestDriverRouteStopsRouteWithoutStopsIsEmpty(t *testing.T) {
	repo := &driverRouteRepo{}
	uc := NewDriverRouteStops(repo)

	got, err := uc.Run(context.Background(), DriverRouteStopsCommand{Role: RoleDriver, RouteID: "route-7"})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("stops = %+v, want none", got)
	}
}
