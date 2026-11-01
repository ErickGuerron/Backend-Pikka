package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestNormalizeCityTrimsAndAcceptsTwoToEightyCharacters(t *testing.T) {
	tests := map[string]string{
		"  Quito  ":                "Quito",
		"Ny":                       "Ny",
		strings.Repeat("a", 80):    strings.Repeat("a", 80),
		"Santo Domingo de los Tsá": "Santo Domingo de los Tsá",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := domain.NormalizeCity(raw)
			if err != nil {
				t.Fatalf("NormalizeCity(%q): %v", raw, err)
			}
			if got != want {
				t.Fatalf("NormalizeCity(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}

func TestNormalizeCityRejectsInvalidValues(t *testing.T) {
	tests := map[string]string{
		"empty":         "",
		"one character": "Q",
		"blank":         "   ",
		"too long":      strings.Repeat("a", 81),
		"embedded NUL":  "Qu\x00ito",
		"invalid UTF-8": "Qu\xffito",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NormalizeCity(raw)
			if !errors.Is(err, domain.ErrInvalidCity) {
				t.Fatalf("err = %v, want ErrInvalidCity", err)
			}
		})
	}
}

func TestWithContactStoresNormalizedPhoneAndCity(t *testing.T) {
	c := domain.NewCustomer(uuid.New(), "ana@example.com", "Ana Pérez")

	got, err := c.WithContact("+593 99-123 4567", "  Quito ")

	if err != nil {
		t.Fatalf("WithContact: %v", err)
	}
	if got.Phone != "+593991234567" || got.City != "Quito" {
		t.Fatalf("contact = %q / %q", got.Phone, got.City)
	}
	if c.Phone != "" || c.City != "" {
		t.Fatalf("WithContact must return a copy, original changed to %q / %q", c.Phone, c.City)
	}
}

func TestWithContactRejectsInvalidPhoneOrCity(t *testing.T) {
	c := domain.NewCustomer(uuid.New(), "ana@example.com", "Ana Pérez")

	if _, err := c.WithContact("abc", "Quito"); !errors.Is(err, domain.ErrInvalidPhone) {
		t.Fatalf("invalid phone: err = %v, want ErrInvalidPhone", err)
	}
	if _, err := c.WithContact("0991234567", "Q"); !errors.Is(err, domain.ErrInvalidCity) {
		t.Fatalf("invalid city: err = %v, want ErrInvalidCity", err)
	}
}
