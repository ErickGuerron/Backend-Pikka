package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Límites del catálogo (ADR 0009).
const (
	MaxProductNameRunes        = 120
	MaxProductDescriptionRunes = 500
	// MaxLineQuantity es la cantidad máxima de un mismo producto en un pedido.
	MaxLineQuantity = 50
)

// Product es un artículo del catálogo. Su stock es opcional: nil significa que el producto no
// controla existencias (por ejemplo, un servicio).
type Product struct {
	ID          uuid.UUID
	Name        string
	Description string
	// UnitPrice es el precio actual en USD con dos decimales como máximo.
	UnitPrice decimal.Decimal
	Active    bool
	// StockQuantity es nil cuando el producto no controla stock.
	StockQuantity *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewProductInput son los datos que el operador envía para crear un producto.
type NewProductInput struct {
	Name        string
	Description string
	UnitPrice   decimal.Decimal
	// InitialStock nil: sin control de stock. Un valor no negativo: stock inicial.
	InitialStock *int64
}

// NewProduct valida la entrada y crea un producto activo. Si hay stock inicial, devuelve también
// el movimiento RESTOCK que lo registra, para que el historial empiece con el primer dato real.
func NewProduct(id uuid.UUID, in NewProductInput, actor *uuid.UUID, now time.Time) (Product, *StockMovement, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxProductNameRunes {
		return Product{}, nil, ErrInvalidProductName
	}
	description := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(description) > MaxProductDescriptionRunes {
		return Product{}, nil, ErrInvalidProductDescription
	}
	if err := ValidatePrice(in.UnitPrice); err != nil {
		return Product{}, nil, err
	}
	p := Product{
		ID:          id,
		Name:        name,
		Description: description,
		UnitPrice:   in.UnitPrice,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if in.InitialStock == nil {
		return p, nil, nil
	}
	if *in.InitialStock < 0 {
		return Product{}, nil, ErrInvalidStock
	}
	stock := *in.InitialStock
	p.StockQuantity = &stock
	movement := StockMovement{
		ID:             uuid.New(),
		ProductID:      id,
		Type:           MovementRestock,
		Delta:          stock,
		ResultingStock: stock,
		Reason:         "stock inicial",
		ActorUserID:    actor,
		CreatedAt:      now,
	}
	return p, &movement, nil
}

// ValidatePrice acepta precios positivos con como máximo dos decimales.
func ValidatePrice(price decimal.Decimal) error {
	if !price.IsPositive() || price.Exponent() < -2 {
		return ErrInvalidPrice
	}
	return nil
}

// ApplyMovement aplica un movimiento de stock a un producto que controla existencias. No puede dejar
// el stock en negativo. Devuelve el movimiento ya calculado, listo para guardar en el historial.
func (p *Product) ApplyMovement(movementType MovementType, delta int64, reason string, orderID *uuid.UUID, actor *uuid.UUID, now time.Time) (StockMovement, error) {
	if p.StockQuantity == nil {
		return StockMovement{}, ErrStockNotTracked
	}
	if err := validateMovement(movementType, delta, reason); err != nil {
		return StockMovement{}, err
	}
	resulting := *p.StockQuantity + delta
	if resulting < 0 {
		return StockMovement{}, ErrInsufficientStock
	}
	*p.StockQuantity = resulting
	p.UpdatedAt = now
	return StockMovement{
		ID:             uuid.New(),
		ProductID:      p.ID,
		Type:           movementType,
		Delta:          delta,
		ResultingStock: resulting,
		Reason:         strings.TrimSpace(reason),
		OrderID:        orderID,
		ActorUserID:    actor,
		CreatedAt:      now,
	}, nil
}

// Deactivate retira el producto de la oferta al cliente. Los pedidos anteriores no cambian.
func (p *Product) Deactivate(now time.Time) {
	p.Active = false
	p.UpdatedAt = now
}

// Activate vuelve a ofrecer el producto al cliente.
func (p *Product) Activate(now time.Time) {
	p.Active = true
	p.UpdatedAt = now
}
