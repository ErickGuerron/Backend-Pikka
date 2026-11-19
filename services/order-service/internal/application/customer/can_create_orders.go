package customer

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
)

// CanCreateOrders es el caso de uso que el Order Service (BE-T5) usará para
// autorizar la creación de un pedido: dada la identidad del Customer,
// devuelve si está habilitado para abrir una orden nueva.
//
// Delegar en el dominio (no en la capa de aplicación) garantiza que la regla
// de negocio viva en un solo lugar.
type CanCreateOrders struct {
	repo ports.CustomerRepository
}

// NewCanCreateOrders inyecta el repositorio.
func NewCanCreateOrders(repo ports.CustomerRepository) *CanCreateOrders {
	return &CanCreateOrders{repo: repo}
}

// Run devuelve (true, nil) si el cliente está ACTIVO, (false, nil) si está
// bloqueado por algún motivo y (false, ErrCustomerNotFound) si no existe.
func (uc *CanCreateOrders) Run(ctx context.Context, id uuid.UUID) (bool, error) {
	c, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	return c.CanCreateOrders(), nil
}
