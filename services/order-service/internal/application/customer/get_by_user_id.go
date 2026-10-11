package customer

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// GetByUserID recupera el Customer asociado al user_id de un usuario
// autenticado en auth-service.
//
// Es la operación primaria de lectura desde el gateway: cada request trae un
// JWT con el user_id y el servicio de órdenes lo cruza con la tabla
// customers de este esquema.
type GetByUserID struct {
	repo ports.CustomerRepository
}

// NewGetByUserID inyecta el repositorio.
func NewGetByUserID(repo ports.CustomerRepository) *GetByUserID {
	return &GetByUserID{repo: repo}
}

// Run devuelve el Customer o domain.ErrCustomerNotFound si el user_id no
// tiene un customer asociado todavía.
func (uc *GetByUserID) Run(ctx context.Context, userID uuid.UUID) (domain.Customer, error) {
	return uc.repo.GetByUserID(ctx, userID)
}
