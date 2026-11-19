package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// StopsBeforeCommand pide cuántas paradas quedan antes de un pedido del cliente autenticado.
type StopsBeforeCommand struct {
	UserID  uuid.UUID
	Role    string
	OrderID string
}

// StopsBeforeForOrder devuelve cuántas paradas abiertas quedan antes del pedido del cliente (ADR 0007, ADR 0008).
// Solo expone un número: el cliente no recibe ids ni estados de pedidos de otros clientes.
type StopsBeforeForOrder struct {
	customers ports.CustomerRepository
	stops     ports.RouteStopRepository
}

// NewStopsBeforeForOrder inyecta los repositorios.
func NewStopsBeforeForOrder(customers ports.CustomerRepository, stops ports.RouteStopRepository) *StopsBeforeForOrder {
	return &StopsBeforeForOrder{customers: customers, stops: stops}
}

// Run valida el rol, resuelve el cliente y cuenta las paradas del pedido propio.
func (uc *StopsBeforeForOrder) Run(ctx context.Context, cmd StopsBeforeCommand) (int, error) {
	if cmd.Role != RoleCustomer {
		return 0, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return 0, err
	}
	return uc.stops.CountStopsBefore(ctx, cmd.OrderID, customer.ID)
}
