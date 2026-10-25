package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Plan (BOLA): a customer reading another customer's order gets "not found", never the order.
func TestPlan_GetMyOrder_AnotherCustomersOrder_IsNotFound(t *testing.T) {
	uc, repo, order := getMyOrderFixture(t)
	other := domain.Customer{ID: uuid.New(), UserID: uuid.New(), Status: activeCustomer().Status}
	uc.customers = &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{other.UserID: other}}

	got, err := uc.Run(context.Background(), GetCommand{UserID: other.UserID, Role: RoleCustomer, OrderID: order.ID})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
	if got.ID != "" {
		t.Fatalf("leaked order %q to another customer", got.ID)
	}
	if repo.calledWith != other.ID {
		t.Fatalf("repository scoped by %s, want the authenticated customer %s", repo.calledWith, other.ID)
	}
}

// Plan (BOLA): cancelling another customer's order is "not found" and leaves the order untouched.
func TestPlan_CancelOrder_AnotherCustomersOrder_IsNotFoundAndUnchanged(t *testing.T) {
	uc, repo := cancelFixture(t, domain.StatusPending)
	other := domain.Customer{ID: uuid.New(), UserID: uuid.New(), Status: activeCustomer().Status}
	uc.customers = &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{other.UserID: other}}

	_, err := uc.Run(context.Background(), CancelCommand{UserID: other.UserID, Role: RoleCustomer, OrderID: repo.current.ID})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
	if repo.current.Status != domain.StatusPending {
		t.Fatalf("status = %s, want PENDING (unchanged)", repo.current.Status)
	}
}
