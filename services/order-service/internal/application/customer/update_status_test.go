package customer

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestUpdateStatus_ValidStatus_Updates(t *testing.T) {
	repo := newFakeCustomerRepository()
	create := NewCreateCustomer(repo)
	update := NewUpdateStatus(repo)

	uid := uuid.New()
	created, err := create.Run(context.Background(), uid, "a@b.com", "Test", "0991234567", "Quito")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	updated, err := update.Run(context.Background(), created.ID, domain.StatusBlockedSurcharge)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != domain.StatusBlockedSurcharge {
		t.Fatalf("status: want BLOQUEADO_POR_SALDO got %q", updated.Status)
	}
	// Persistencia real debe leerse también
	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID tras update: %v", err)
	}
	if got.Status != domain.StatusBlockedSurcharge {
		t.Fatalf("status persistido: want BLOQUEADO_POR_SALDO got %q", got.Status)
	}
}

func TestUpdateStatus_InvalidStatus_ReturnsError(t *testing.T) {
	repo := newFakeCustomerRepository()
	create := NewCreateCustomer(repo)
	update := NewUpdateStatus(repo)

	created, err := create.Run(context.Background(), uuid.New(), "a@b.com", "Test", "0991234567", "Quito")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err = update.Run(context.Background(), created.ID, domain.CustomerStatus("NOPE"))
	if err == nil {
		t.Fatal("status inválido debe rechazarse")
	}
	if !errors.Is(err, domain.ErrInvalidStatus) {
		t.Fatalf("want ErrInvalidStatus, got %v", err)
	}
}

func TestUpdateStatus_NotFound_ReturnsError(t *testing.T) {
	repo := newFakeCustomerRepository()
	update := NewUpdateStatus(repo)

	_, err := update.Run(context.Background(), uuid.New(), domain.StatusBlockedSurcharge)
	if err == nil {
		t.Fatal("id desconocido debe fallar")
	}
	if !errors.Is(err, domain.ErrCustomerNotFound) {
		t.Fatalf("want ErrCustomerNotFound, got %v", err)
	}
}
