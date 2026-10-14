package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// CatalogRepository persiste productos y su historial de stock (ADR 0009).
type CatalogRepository interface {
	// CreateProduct guarda el producto y, si tiene stock inicial, su movimiento RESTOCK, en una sola transacción.
	CreateProduct(ctx context.Context, p domain.Product, initial *domain.StockMovement) error
	// GetProduct devuelve ErrProductNotFound si no existe.
	GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error)
	// ListActiveProducts devuelve los productos que el cliente puede pedir, ordenados por nombre.
	ListActiveProducts(ctx context.Context) ([]domain.Product, error)
	// SetActive oferta o retira el producto al cliente. No cambia pedidos anteriores.
	SetActive(ctx context.Context, id uuid.UUID, active bool, now time.Time) error
	// AdjustStock aplica una entrada o un ajuste manual y lo registra en el historial.
	AdjustStock(ctx context.Context, productID uuid.UUID, kind domain.MovementType, delta int64, reason string, actor *uuid.UUID, now time.Time) (domain.StockMovement, error)
}

// OrderPlacer guarda pedidos con sus líneas y descuenta el stock en la misma transacción.
type OrderPlacer interface {
	// PlaceOrder registra el pedido, sus líneas y los movimientos SALE. Si algún producto no tiene
	// stock suficiente, o no existe o está inactivo, no se guarda nada.
	PlaceOrder(ctx context.Context, order domain.Order, lines []domain.OrderLine, actor *uuid.UUID) (domain.Order, error)
}
