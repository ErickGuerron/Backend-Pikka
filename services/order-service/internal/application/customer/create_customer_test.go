package customer

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestCreateCustomer_Valid_ReturnsCustomer(t *testing.T) {
	repo := newFakeCustomerRepository()
	uc := NewCreateCustomer(repo)

	uid := uuid.New()
	got, err := uc.Run(context.Background(), uid, "a@b.com", "Test User")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if got.ID == uuid.Nil {
		t.Fatal("el repositorio debe asignar ID")
	}
	if got.UserID != uid {
		t.Fatalf("user_id no se preservó: want %v got %v", uid, got.UserID)
	}
	if got.Status != domain.StatusActive {
		t.Fatalf("status inicial debe ser ACTIVO, got %q", got.Status)
	}
}

func TestCreateCustomer_DuplicateUserID_ReturnsError(t *testing.T) {
	repo := newFakeCustomerRepository()
	uc := NewCreateCustomer(repo)

	uid := uuid.New()
	if _, err := uc.Run(context.Background(), uid, "a@b.com", "Test"); err != nil {
		t.Fatalf("primer alta: %v", err)
	}
	_, err := uc.Run(context.Background(), uid, "a@b.com", "Test 2")
	if err == nil {
		t.Fatal("segundo alta con mismo user_id debe fallar")
	}
	if !errors.Is(err, domain.ErrCustomerAlreadyExists) {
		t.Fatalf("want ErrCustomerAlreadyExists, got %v", err)
	}
}
