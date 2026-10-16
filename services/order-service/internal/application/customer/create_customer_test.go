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
	got, err := uc.Run(context.Background(), uid, "a@b.com", "Test User", "099 123-4567", "  Quito ")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if got.Phone != "0991234567" || got.City != "Quito" {
		t.Fatalf("contacto debe guardarse normalizado, got phone=%q city=%q", got.Phone, got.City)
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
	if _, err := uc.Run(context.Background(), uid, "a@b.com", "Test", "0991234567", "Quito"); err != nil {
		t.Fatalf("primer alta: %v", err)
	}
	_, err := uc.Run(context.Background(), uid, "a@b.com", "Test 2", "0991234567", "Quito")
	if err == nil {
		t.Fatal("segundo alta con mismo user_id debe fallar")
	}
	if !errors.Is(err, domain.ErrCustomerAlreadyExists) {
		t.Fatalf("want ErrCustomerAlreadyExists, got %v", err)
	}
}

func TestCreateCustomer_InvalidContact_IsRejectedBeforePersisting(t *testing.T) {
	tests := []struct {
		name        string
		phone, city string
		want        error
	}{
		{"empty phone", "", "Quito", domain.ErrInvalidPhone},
		{"letters in phone", "abc1234567", "Quito", domain.ErrInvalidPhone},
		{"empty city", "0991234567", "  ", domain.ErrInvalidCity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewCreateCustomer(newFakeCustomerRepository())
			_, err := uc.Run(context.Background(), uuid.New(), "a@b.com", "Test", tt.phone, tt.city)
			if !errors.Is(err, tt.want) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
		})
	}
}
