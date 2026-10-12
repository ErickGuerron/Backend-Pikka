package customer

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestCanCreateOrders_Active_ReturnsTrue(t *testing.T) {
	repo := newFakeCustomerRepository()
	create := NewCreateCustomer(repo)
	can := NewCanCreateOrders(repo)

	created, err := create.Run(context.Background(), uuid.New(), "a@b.com", "Test")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	ok, err := can.Run(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("CanCreateOrders: %v", err)
	}
	if !ok {
		t.Fatal("customer ACTIVO debe poder crear pedidos")
	}
}

func TestCanCreateOrders_Blocked_ReturnsFalse(t *testing.T) {
	repo := newFakeCustomerRepository()
	create := NewCreateCustomer(repo)
	update := NewUpdateStatus(repo)
	can := NewCanCreateOrders(repo)

	created, err := create.Run(context.Background(), uuid.New(), "a@b.com", "Test")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, blocked := range []domain.CustomerStatus{domain.StatusBlockedSurcharge, domain.StatusBlockedIncidents} {
		if _, err := update.Run(context.Background(), created.ID, blocked); err != nil {
			t.Fatalf("update a %s: %v", blocked, err)
		}
		ok, err := can.Run(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("CanCreateOrders(%s): %v", blocked, err)
		}
		if ok {
			t.Fatalf("customer %s NO debe poder crear pedidos", blocked)
		}
	}
}

func TestCanCreateOrders_NotFound_ReturnsError(t *testing.T) {
	repo := newFakeCustomerRepository()
	can := NewCanCreateOrders(repo)

	_, err := can.Run(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("id desconocido debe fallar")
	}
	if !errors.Is(err, domain.ErrCustomerNotFound) {
		t.Fatalf("want ErrCustomerNotFound, got %v", err)
	}
}
