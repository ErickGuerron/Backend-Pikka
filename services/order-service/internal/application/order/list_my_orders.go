package order

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ListMyOrders devuelve los pedidos del cliente autenticado. El alcance lo fija
// la identidad: un cliente nunca ve pedidos ajenos (BOLA, ADR 0007).
type ListMyOrders struct {
	customers ports.CustomerRepository
	orders    ports.OrderRepository
	queue     *ZoneQueue
}

// NewListMyOrders inyecta los repositorios.
func NewListMyOrders(customers ports.CustomerRepository, orders ports.OrderRepository) *ListMyOrders {
	return &ListMyOrders{customers: customers, orders: orders}
}

// WithZoneQueue adjunta a cada pedido en espera su lugar en la cola de su zona.
func (uc *ListMyOrders) WithZoneQueue(queue *ZoneQueue) *ListMyOrders {
	uc.queue = queue
	return uc
}

// Run lista los pedidos del cliente del usuario indicado.
func (uc *ListMyOrders) Run(ctx context.Context, cmd Command) ([]domain.Order, error) {
	if cmd.Role != RoleCustomer {
		return nil, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return nil, err
	}
	orders, err := uc.orders.ListByCustomer(ctx, customer.ID)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		position, err := uc.queue.positionOf(ctx, orders[i])
		if err != nil {
			return nil, err
		}
		orders[i].QueuePosition = position
	}
	return orders, nil
}
