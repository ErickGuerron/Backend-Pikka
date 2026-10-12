// Package ports define los contratos (interfaces) que la capa de aplicación
// necesita de los adaptadores. Permiten que el dominio y los casos de uso
// sean agnósticos de PostgreSQL, gRPC, HTTP, etc.
package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// CustomerRepository es el contrato de persistencia de Customer.
// Las implementaciones viven en internal/adapters/persistence.
type CustomerRepository interface {
	// Create persiste un Customer nuevo. Devuelve ErrCustomerAlreadyExists
	// si el user_id ya está registrado.
	Create(ctx context.Context, c domain.Customer) (domain.Customer, error)

	// GetByUserID busca al cliente asociado a un user_id del esquema auth.
	// Devuelve ErrCustomerNotFound si no existe.
	GetByUserID(ctx context.Context, userID uuid.UUID) (domain.Customer, error)

	// GetByID busca por PK. Devuelve ErrCustomerNotFound si no existe.
	GetByID(ctx context.Context, id uuid.UUID) (domain.Customer, error)

	// UpdateStatus cambia el CustomerStatus de un cliente existente.
	// Devuelve ErrInvalidStatus si el status no es válido y
	// ErrCustomerNotFound si el cliente no existe.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.CustomerStatus) (domain.Customer, error)
}
