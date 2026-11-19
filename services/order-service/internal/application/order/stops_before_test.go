package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// stopsBeforeRepo cuenta las paradas de un pedido del cliente. Registra con qué pedido y cliente se pidió.
type stopsBeforeRepo struct {
	count       int
	err         error
	calls       int
	gotOrder    string
	gotCustomer uuid.UUID
}

func (r *stopsBeforeRepo) CountStopsBefore(_ context.Context, orderID string, customerID uuid.UUID) (int, error) {
	r.calls++
	r.gotOrder, r.gotCustomer = orderID, customerID
	return r.count, r.err
}

func stopsBeforeFixture(repo *stopsBeforeRepo) *StopsBeforeForOrder {
	customer := activeCustomer()
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	return NewStopsBeforeForOrder(customers, repo)
}

func TestStopsBeforeForOrderCustomerGetsCountForOwnOrder(t *testing.T) {
	repo := &stopsBeforeRepo{count: 2}
	uc := stopsBeforeFixture(repo)
	orderID := uuid.NewString()

	got, err := uc.Run(context.Background(), StopsBeforeCommand{UserID: customerUser, Role: RoleCustomer, OrderID: orderID})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 2 {
		t.Fatalf("stops before = %d, want 2", got)
	}
	if repo.gotOrder != orderID {
		t.Fatalf("order = %q, want %q", repo.gotOrder, orderID)
	}
	if repo.gotCustomer != customerID {
		t.Fatalf("repository got customer %s, want the customer profile id, not the user id", repo.gotCustomer)
	}
}

func TestStopsBeforeForOrderForeignOrderIsNotFound(t *testing.T) {
	repo := &stopsBeforeRepo{err: domain.ErrOrderNotFound}
	uc := stopsBeforeFixture(repo)

	_, err := uc.Run(context.Background(), StopsBeforeCommand{UserID: customerUser, Role: RoleCustomer, OrderID: uuid.NewString()})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound (BOLA: no 403, no leak)", err)
	}
}

func TestStopsBeforeForOrderRejectsNonCustomerRoleBeforeRepository(t *testing.T) {
	for _, role := range []string{"", RoleOperator, RoleDriver, RoleAdmin} {
		repo := &stopsBeforeRepo{count: 5}
		uc := stopsBeforeFixture(repo)

		_, err := uc.Run(context.Background(), StopsBeforeCommand{UserID: customerUser, Role: role, OrderID: uuid.NewString()})

		if !errors.Is(err, domain.ErrNotCustomer) {
			t.Fatalf("role %q: err = %v, want ErrNotCustomer", role, err)
		}
		if repo.calls != 0 {
			t.Fatalf("role %q reached the repository", role)
		}
	}
}

func TestStopsBeforeForOrderUserWithoutCustomerProfileIsRejected(t *testing.T) {
	repo := &stopsBeforeRepo{count: 5}
	uc := stopsBeforeFixture(repo)

	_, err := uc.Run(context.Background(), StopsBeforeCommand{UserID: uuid.New(), Role: RoleCustomer, OrderID: uuid.NewString()})

	if !errors.Is(err, domain.ErrCustomerNotFound) {
		t.Fatalf("err = %v, want ErrCustomerNotFound", err)
	}
	if repo.calls != 0 {
		t.Fatal("a user without a customer profile reached the repository")
	}
}
