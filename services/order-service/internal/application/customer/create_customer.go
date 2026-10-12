package customer

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// CreateCustomer es el caso de uso que da de alta un Customer en el esquema
// orders a partir de un user_id recién creado en auth-service.
//
// El dominio garantiza el estado inicial (Status=ACTIVO,
// PendingSurcharge=0). El puerto se encarga de la unicidad (UNIQUE INDEX
// sobre user_id) y devuelve ErrCustomerAlreadyExists si el user_id está
// repetido.
type CreateCustomer struct {
	repo ports.CustomerRepository
}

// NewCreateCustomer inyecta el repositorio (inyección por constructor).
func NewCreateCustomer(repo ports.CustomerRepository) *CreateCustomer {
	return &CreateCustomer{repo: repo}
}

// Run ejecuta el alta y devuelve el Customer persistido (con ID y timestamps
// asignados por la capa de persistencia).
func (uc *CreateCustomer) Run(ctx context.Context, userID uuid.UUID, email, fullName string) (domain.Customer, error) {
	c := domain.NewCustomer(userID, email, fullName)
	return uc.repo.Create(ctx, c)
}
