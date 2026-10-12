package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCustomer_CanCreateOrders_DelegatesToStatus(t *testing.T) {
	active := Customer{Status: StatusActive}
	if !active.CanCreateOrders() {
		t.Fatal("Customer con status ACTIVO debe poder crear pedidos")
	}
	blocked := Customer{Status: StatusBlockedSurcharge}
	if blocked.CanCreateOrders() {
		t.Fatal("Customer bloqueado NO debe poder crear pedidos")
	}
}

func TestCustomer_New_DefaultsToActive(t *testing.T) {
	uid := uuid.New()
	c := NewCustomer(uid, "cliente@x.com", "Nombre Apellido")
	if c.UserID != uid {
		t.Fatalf("user_id: want %v got %v", uid, c.UserID)
	}
	if c.Email != "cliente@x.com" {
		t.Fatalf("email: want %q got %q", "cliente@x.com", c.Email)
	}
	if c.FullName != "Nombre Apellido" {
		t.Fatalf("full_name: want %q got %q", "Nombre Apellido", c.FullName)
	}
	if c.Status != StatusActive {
		t.Fatalf("status por defecto debe ser ACTIVO, obtuve %q", c.Status)
	}
	if !c.PendingSurcharge.Equal(decimal.Zero) {
		t.Fatalf("pending_surcharge por defecto debe ser 0, obtuve %s", c.PendingSurcharge.String())
	}
}

func TestCustomer_AddSurcharge_Accumulates(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	if err := c.AddSurcharge(decimal.RequireFromString("10.50")); err != nil {
		t.Fatalf("primer AddSurcharge: %v", err)
	}
	if err := c.AddSurcharge(decimal.RequireFromString("4.25")); err != nil {
		t.Fatalf("segundo AddSurcharge: %v", err)
	}
	want := decimal.RequireFromString("14.75")
	if !c.PendingSurcharge.Equal(want) {
		t.Fatalf("pending_surcharge acumulado: want %s got %s", want.String(), c.PendingSurcharge.String())
	}
}

func TestCustomer_AddSurcharge_RejectsNegative(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	err := c.AddSurcharge(decimal.RequireFromString("-1.00"))
	if err == nil {
		t.Fatal("AddSurcharge con monto negativo debe devolver error")
	}
	if !errors.Is(err, ErrInvalidSurcharge) {
		t.Fatalf("want ErrInvalidSurcharge, got %v", err)
	}
}

func TestCustomer_PaySurcharge_ReducesBalance(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	if err := c.AddSurcharge(decimal.RequireFromString("20.00")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := c.PaySurcharge(decimal.RequireFromString("7.50")); err != nil {
		t.Fatalf("PaySurcharge: %v", err)
	}
	want := decimal.RequireFromString("12.50")
	if !c.PendingSurcharge.Equal(want) {
		t.Fatalf("saldo: want %s got %s", want.String(), c.PendingSurcharge.String())
	}
}

func TestCustomer_PaySurcharge_RejectsNonPositive(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	if err := c.AddSurcharge(decimal.RequireFromString("5.00")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := c.PaySurcharge(decimal.Zero); !errors.Is(err, ErrInvalidSurcharge) {
		t.Fatalf("pago cero debe rechazarse: %v", err)
	}
	if err := c.PaySurcharge(decimal.RequireFromString("-1.00")); !errors.Is(err, ErrInvalidSurcharge) {
		t.Fatalf("pago negativo debe rechazarse: %v", err)
	}
	if !c.PendingSurcharge.Equal(decimal.RequireFromString("5.00")) {
		t.Fatal("el saldo no debe cambiar en pagos rechazados")
	}
}

func TestCustomer_PaySurcharge_OverpayRejected(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	if err := c.AddSurcharge(decimal.RequireFromString("5.00")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := c.PaySurcharge(decimal.RequireFromString("6.00"))
	if err == nil {
		t.Fatal("pago mayor al saldo debe rechazarse")
	}
	if !errors.Is(err, ErrInvalidSurcharge) {
		t.Fatalf("want ErrInvalidSurcharge, got %v", err)
	}
	if !c.PendingSurcharge.Equal(decimal.RequireFromString("5.00")) {
		t.Fatal("el saldo no debe cambiar en pagos rechazados")
	}
}

func TestCustomer_BlockBySurcharge_UpdatesStatus(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	c.BlockBySurcharge()
	if c.Status != StatusBlockedSurcharge {
		t.Fatalf("status: want BLOQUEADO_POR_SALDO got %q", c.Status)
	}
}

func TestCustomer_Unblock_ResetsToActive(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	c.BlockBySurcharge()
	c.Unblock()
	if c.Status != StatusActive {
		t.Fatalf("status: want ACTIVO got %q", c.Status)
	}
}

func TestCustomer_UnblockIfPaid_RequiresZeroBalance(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	c.BlockBySurcharge()
	if err := c.AddSurcharge(decimal.RequireFromString("10")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := c.UnblockIfPaid()
	if err == nil {
		t.Fatal("UnblockIfPaid con saldo pendiente debe devolver error")
	}
	if !errors.Is(err, ErrPendingSurcharge) {
		t.Fatalf("want ErrPendingSurcharge, got %v", err)
	}
	if c.Status != StatusBlockedSurcharge {
		t.Fatalf("status no debe cambiar cuando hay saldo: %q", c.Status)
	}

	if err := c.PaySurcharge(decimal.RequireFromString("10")); err != nil {
		t.Fatalf("setup pay: %v", err)
	}
	if err := c.UnblockIfPaid(); err != nil {
		t.Fatalf("UnblockIfPaid con saldo 0: %v", err)
	}
	if c.Status != StatusActive {
		t.Fatalf("status: want ACTIVO got %q", c.Status)
	}
}

func TestCustomer_Touch_UpdatesTimestamp(t *testing.T) {
	c := NewCustomer(uuid.New(), "a@b.com", "Test")
	before := time.Now().Add(-time.Second)
	c.touch()
	if !c.UpdatedAt.After(before) {
		t.Fatalf("UpdatedAt debe avanzar al tocar; got %v", c.UpdatedAt)
	}
}
