package customer

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestGetByUserID_NotFound_ReturnsError(t *testing.T) {
	repo := newFakeCustomerRepository()
	uc := NewGetByUserID(repo)

	_, err := uc.Run(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("user_id desconocido debe devolver error")
	}
	if !errors.Is(err, domain.ErrCustomerNotFound) {
		t.Fatalf("want ErrCustomerNotFound, got %v", err)
	}
}

func TestGetByUserID_ReturnsInserted(t *testing.T) {
	repo := newFakeCustomerRepository()
	create := NewCreateCustomer(repo)
	get := NewGetByUserID(repo)

	uid := uuid.New()
	created, err := create.Run(context.Background(), uid, "a@b.com", "Test", "0991234567", "Quito")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := get.Run(context.Background(), uid)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("id: want %v got %v", created.ID, got.ID)
	}
}
