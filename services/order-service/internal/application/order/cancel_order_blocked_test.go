package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func cancelWithStatus(status domain.CustomerStatus) (*CancelOrder, *cancelRepo, domain.Customer) {
	customer := activeCustomer()
	customer.Status = status
	order := domain.Order{ID: uuid.NewString(), CustomerID: customer.ID.String(), Status: domain.StatusPending}
	repo := &cancelRepo{current: order}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	return NewCancelOrder(customers, repo), repo, customer
}

// ADR 0006 sección 5: la cuenta bloqueada por saldo no puede cancelar pedidos.
func TestCancelOrderBlockedBySurchargeIsRejectedBeforeTheRepository(t *testing.T) {
	uc, repo, customer := cancelWithStatus(domain.StatusBlockedSurcharge)

	_, err := uc.Run(context.Background(), CancelCommand{UserID: customer.UserID, Role: RoleCustomer, OrderID: repo.current.ID})

	if !errors.Is(err, domain.ErrCustomerBlockedBySurcharge) {
		t.Fatalf("err = %v, want ErrCustomerBlockedBySurcharge", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository called %d times", repo.calls)
	}
}

// Existing behavior: only the balance block stops cancelling; the other statuses keep cancelling.
func TestCancelOrderStillWorksForActiveAndIncidentBlockedCustomers(t *testing.T) {
	for _, status := range []domain.CustomerStatus{domain.StatusActive, domain.StatusBlockedIncidents} {
		uc, repo, customer := cancelWithStatus(status)

		got, err := uc.Run(context.Background(), CancelCommand{UserID: customer.UserID, Role: RoleCustomer, OrderID: repo.current.ID})

		if err != nil || got.Status != domain.StatusCancelled {
			t.Fatalf("%s: status = %s, err = %v", status, got.Status, err)
		}
	}
}
