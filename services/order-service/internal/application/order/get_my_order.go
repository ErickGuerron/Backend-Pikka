package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// GetCommand pide un pedido concreto. La identidad viene de la metadata del Gateway.
type GetCommand struct {
	UserID  uuid.UUID
	Role    string
	OrderID string
}

// GetMyOrder devuelve un pedido del cliente autenticado (ADR 0007, ADR 0008). El alcance lo fija
// la consulta por cliente: un pedido ajeno responde como inexistente, nunca como prohibido.
type GetMyOrder struct {
	customers ports.CustomerRepository
	orders    ports.OrderRepository
	queue     *ZoneQueue
}

// NewGetMyOrder inyecta los repositorios.
func NewGetMyOrder(customers ports.CustomerRepository, orders ports.OrderRepository) *GetMyOrder {
	return &GetMyOrder{customers: customers, orders: orders}
}

// WithZoneQueue adjunta al pedido en espera su lugar en la cola de su zona.
func (uc *GetMyOrder) WithZoneQueue(queue *ZoneQueue) *GetMyOrder {
	uc.queue = queue
	return uc
}

// Run devuelve el pedido indicado si pertenece al cliente autenticado.
func (uc *GetMyOrder) Run(ctx context.Context, cmd GetCommand) (domain.Order, error) {
	if cmd.Role != RoleCustomer {
		return domain.Order{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return domain.Order{}, err
	}
	order, err := uc.orders.GetForCustomer(ctx, cmd.OrderID, customer.ID)
	if err != nil {
		return domain.Order{}, err
	}
	position, err := uc.queue.positionOf(ctx, order)
	if err != nil {
		return domain.Order{}, err
	}
	order.QueuePosition = position
	return order, nil
}
