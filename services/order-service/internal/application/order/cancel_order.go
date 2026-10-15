package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// CancelCommand pide la cancelación de un pedido. La identidad viene de la metadata del Gateway.
type CancelCommand struct {
	UserID  uuid.UUID
	Role    string
	OrderID string
}

// CancelOrder cancela un pedido del cliente autenticado (ADR 0007, sección 8). El alcance lo fija
// la identidad: un pedido ajeno responde como inexistente.
type CancelOrder struct {
	customers ports.CustomerRepository
	orders    ports.OrderRepository
}

// NewCancelOrder inyecta los repositorios.
func NewCancelOrder(customers ports.CustomerRepository, orders ports.OrderRepository) *CancelOrder {
	return &CancelOrder{customers: customers, orders: orders}
}

// Run cancela el pedido si pertenece al cliente y su estado lo permite.
func (uc *CancelOrder) Run(ctx context.Context, cmd CancelCommand) (domain.Order, error) {
	if cmd.Role != RoleCustomer {
		return domain.Order{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return domain.Order{}, err
	}
	return uc.orders.CancelForCustomer(ctx, cmd.OrderID, customer.ID, func(current domain.Order) (domain.Order, error) {
		return current.CancelByCustomer()
	})
}
