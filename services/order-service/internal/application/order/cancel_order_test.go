package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// cancelRepo aplica la decisión sobre el pedido guardado, como lo hace la transacción real,
// y registra con qué cliente se pidió la cancelación.
type cancelRepo struct {
	ports.OrderRepository
	current    domain.Order
	calledWith uuid.UUID
	calls      int
}

func (r *cancelRepo) CancelForCustomer(_ context.Context, orderID string, customerID uuid.UUID, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	r.calls++
	r.calledWith = customerID
	if orderID != r.current.ID || customerID.String() != r.current.CustomerID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	next, err := decide(r.current)
	if err != nil {
		return domain.Order{}, err
	}
	r.current = next
	return next, nil
}

func cancelFixture(t *testing.T, status domain.Status) (*CancelOrder, *cancelRepo) {
	t.Helper()
	customer := activeCustomer()
	order := domain.Order{ID: uuid.NewString(), CustomerID: customer.ID.String(), Status: status}
	repo := &cancelRepo{current: order}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	return NewCancelOrder(customers, repo), repo
}

func TestCancelOrderCustomerCancelsOwnPendingOrder(t *testing.T) {
	uc, repo := cancelFixture(t, domain.StatusPending)

	got, err := uc.Run(context.Background(), CancelCommand{
		UserID:  activeCustomer().UserID,
		Role:    RoleCustomer,
		OrderID: repo.current.ID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", got.Status)
	}
	if repo.calledWith != activeCustomer().ID {
		t.Fatalf("repository got customer %s, want the authenticated customer", repo.calledWith)
	}
}

func TestCancelOrderRejectsNonCustomerRole(t *testing.T) {
	uc, repo := cancelFixture(t, domain.StatusPending)

	_, err := uc.Run(context.Background(), CancelCommand{
		UserID:  activeCustomer().UserID,
		Role:    RoleOperator,
		OrderID: repo.current.ID,
	})
	if !errors.Is(err, domain.ErrNotCustomer) {
		t.Fatalf("err = %v, want ErrNotCustomer", err)
	}
	if repo.calls != 0 {
		t.Fatal("a non-customer must not reach the repository")
	}
}

func TestCancelOrderOfAnotherCustomerIsNotFound(t *testing.T) {
	uc, repo := cancelFixture(t, domain.StatusPending)
	other := domain.Customer{ID: uuid.New(), UserID: uuid.New(), Status: activeCustomer().Status}
	uc.customers = &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{other.UserID: other}}

	_, err := uc.Run(context.Background(), CancelCommand{
		UserID:  other.UserID,
		Role:    RoleCustomer,
		OrderID: repo.current.ID,
	})
	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound (BOLA: no 403, no leak)", err)
	}
	if repo.current.Status != domain.StatusPending {
		t.Fatal("another customer's order must not change")
	}
}

func TestCancelOrderRejectsStatesAfterReview(t *testing.T) {
	uc, repo := cancelFixture(t, domain.StatusPickedUp)

	_, err := uc.Run(context.Background(), CancelCommand{
		UserID:  activeCustomer().UserID,
		Role:    RoleCustomer,
		OrderID: repo.current.ID,
	})
	if !errors.Is(err, domain.ErrOrderNotCancellable) {
		t.Fatalf("err = %v, want ErrOrderNotCancellable", err)
	}
	if repo.current.Status != domain.StatusPickedUp {
		t.Fatalf("status = %s, want PICKED_UP unchanged", repo.current.Status)
	}
}
