package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// MovementType clasifica un cambio de stock (ADR 0009, sección 2).
type MovementType string

const (
	// MovementRestock es una entrada de mercadería. Siempre suma.
	MovementRestock MovementType = "RESTOCK"
	// MovementAdjustment es una corrección manual con motivo; puede sumar o restar.
	MovementAdjustment MovementType = "ADJUSTMENT"
	// MovementSale descuenta al crear un pedido. Siempre resta.
	MovementSale MovementType = "SALE"
	// MovementReturn devuelve stock al cancelar un pedido. Siempre suma.
	MovementReturn MovementType = "RETURN"
)

// StockMovement es una entrada inmutable del historial de un producto.
type StockMovement struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	Type      MovementType
	// Delta es la variación con signo: positiva suma y negativa resta.
	Delta int64
	// ResultingStock es el stock después de aplicar el movimiento.
	ResultingStock int64
	Reason         string
	OrderID        *uuid.UUID
	ActorUserID    *uuid.UUID
	CreatedAt      time.Time
}

// validateMovement comprueba que el tipo, el signo del delta y el motivo sean coherentes.
func validateMovement(movementType MovementType, delta int64, reason string) error {
	switch movementType {
	case MovementRestock, MovementReturn:
		if delta <= 0 {
			return ErrInvalidMovement
		}
	case MovementSale:
		if delta >= 0 {
			return ErrInvalidMovement
		}
	case MovementAdjustment:
		if delta == 0 {
			return ErrInvalidMovement
		}
		if strings.TrimSpace(reason) == "" {
			return ErrReasonRequired
		}
	default:
		return ErrInvalidMovement
	}
	return nil
}
