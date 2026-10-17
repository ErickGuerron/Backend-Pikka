package order

import (
	"context"
	"errors"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ErrInvalidDeliveryDate indica que el filtro de fecha no es YYYY-MM-DD.
var ErrInvalidDeliveryDate = errors.New("delivery date must be YYYY-MM-DD")

// ListOrders devuelve los pedidos de todos los clientes para el personal (ADMIN y OPERATOR).
type ListOrders struct {
	orders ports.OrderRepository
}

// NewListOrders inyecta el repositorio.
func NewListOrders(orders ports.OrderRepository) *ListOrders {
	return &ListOrders{orders: orders}
}

// ListOrdersQuery es la identidad del llamador y los filtros opcionales.
type ListOrdersQuery struct {
	Role         string
	DeliveryDate string
	Status       domain.Status
}

// Run lista los pedidos. Solo el personal puede hacerlo; un cliente solo ve los suyos (ListMyOrders).
func (uc *ListOrders) Run(ctx context.Context, q ListOrdersQuery) ([]domain.Order, error) {
	if !isOperator(q.Role) {
		return nil, domain.ErrNotOperator
	}
	if q.DeliveryDate != "" {
		if _, err := time.Parse(time.DateOnly, q.DeliveryDate); err != nil {
			return nil, ErrInvalidDeliveryDate
		}
	}
	return uc.orders.ListAll(ctx, ports.OrderFilter{DeliveryDate: q.DeliveryDate, Status: q.Status})
}
