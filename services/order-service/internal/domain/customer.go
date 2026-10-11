// Package domain contiene las reglas de negocio del esquema orders.
// Es código puro: no depende de PostgreSQL, gRPC ni del framework HTTP.
package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Customer es la entidad de negocio de un cliente en el esquema orders.
//
// Vincula a un usuario del esquema auth por su user_id. No declaramos FK
// hacia auth.users por la restricción del esquema documentada en el ADR 0003
// (el esquema orders no tiene permisos de lectura sobre auth); la integridad
// se garantiza en la capa de aplicación + UNIQUE INDEX en user_id.
type Customer struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	Email    string
	FullName string

	Status           CustomerStatus
	PendingSurcharge decimal.Decimal

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewCustomer construye un Customer nuevo para un user_id recién dado de alta
// en auth. El status se inicializa en ACTIVO y el saldo pendiente en cero.
// La persistencia completa CreatedAt/UpdatedAt.
func NewCustomer(userID uuid.UUID, email, fullName string) Customer {
	return Customer{
		UserID:           userID,
		Email:            email,
		FullName:         fullName,
		Status:           StatusActive,
		PendingSurcharge: decimal.Zero,
	}
}

// CanCreateOrders delega al status. La entidad no aporta lógica adicional.
func (c Customer) CanCreateOrders() bool {
	return c.Status.CanCreateOrders()
}

// AddSurcharge acumula un recargo al saldo pendiente del cliente.
// Rechaza montos no positivos para evitar saldos basura.
func (c *Customer) AddSurcharge(amount decimal.Decimal) error {
	if amount.Sign() <= 0 {
		return ErrInvalidSurcharge
	}
	c.PendingSurcharge = c.PendingSurcharge.Add(amount)
	c.touch()
	return nil
}

// PaySurcharge reduce el saldo pendiente. Rechaza pagos no positivos y
// pagos que excedan el saldo actual (no se admiten saldos negativos).
func (c *Customer) PaySurcharge(amount decimal.Decimal) error {
	if amount.Sign() <= 0 {
		return ErrInvalidSurcharge
	}
	if amount.GreaterThan(c.PendingSurcharge) {
		return ErrInvalidSurcharge
	}
	c.PendingSurcharge = c.PendingSurcharge.Sub(amount)
	c.touch()
	return nil
}

// BlockBySurcharge marca al cliente como bloqueado por saldo pendiente.
// Lo usa el flujo de "cliente llegó al umbral de recargo".
func (c *Customer) BlockBySurcharge() {
	c.Status = StatusBlockedSurcharge
	c.touch()
}

// Unblock restablece al cliente como ACTIVO. Úsalo cuando el motivo del
// bloqueo fue operativo y se resolvió manualmente.
func (c *Customer) Unblock() {
	c.Status = StatusActive
	c.touch()
}

// UnblockIfPaid desbloquea al cliente únicamente si su saldo pendiente es
// cero. Devuelve ErrPendingSurcharge si todavía queda saldo a cobrar.
func (c *Customer) UnblockIfPaid() error {
	if !c.PendingSurcharge.IsZero() {
		return ErrPendingSurcharge
	}
	c.Status = StatusActive
	c.touch()
	return nil
}

// touch avanza UpdatedAt. Existe como helper privado para no arrastrar
// time.Now() por cada operación de dominio.
func (c *Customer) touch() {
	c.UpdatedAt = time.Now()
}
