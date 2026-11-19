package customer

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// GetByID recupera un Customer por su id interno. Se usa en operaciones
// internas del Gateway y de otros servicios cuando ya se conoce el UUID
// del Customer.
type GetByID struct {
	repo ports.CustomerRepository
}

// NewGetByID inyecta el repositorio.
func NewGetByID(repo ports.CustomerRepository) *GetByID {
	return &GetByID{repo: repo}
}

// Run devuelve el Customer o domain.ErrCustomerNotFound si el id no existe.
func (uc *GetByID) Run(ctx context.Context, id uuid.UUID) (domain.Customer, error) {
	return uc.repo.GetByID(ctx, id)
}
