package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OrderLine es un producto dentro de un pedido. Guarda nombre y precio del momento del pedido:
// cambiar el catálogo después no altera pedidos ya registrados (ADR 0009, sección 3).
type OrderLine struct {
	ProductID uuid.UUID
	Name      string
	UnitPrice decimal.Decimal
	Quantity  int64
}

// NewOrderLine copia el producto en la línea y valida la cantidad. Un producto inactivo no puede
// entrar a un pedido nuevo.
func NewOrderLine(p Product, quantity int64) (OrderLine, error) {
	if !p.Active {
		return OrderLine{}, ErrProductInactive
	}
	if quantity < 1 || quantity > MaxLineQuantity {
		return OrderLine{}, ErrInvalidQuantity
	}
	return OrderLine{ProductID: p.ID, Name: p.Name, UnitPrice: p.UnitPrice, Quantity: quantity}, nil
}

// Subtotal es el precio unitario por la cantidad.
func (l OrderLine) Subtotal() decimal.Decimal {
	return l.UnitPrice.Mul(decimal.NewFromInt(l.Quantity))
}

// OrderTotal suma los subtotales. Un pedido sin líneas no tiene total válido.
func OrderTotal(lines []OrderLine) (decimal.Decimal, error) {
	if len(lines) == 0 {
		return decimal.Zero, ErrNoItems
	}
	total := decimal.Zero
	for _, line := range lines {
		total = total.Add(line.Subtotal())
	}
	return total, nil
}
