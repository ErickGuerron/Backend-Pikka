package customer

import (
	"context"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// UpdateStatus cambia el estado de un Customer.
// Se utiliza desde flujos internos:
//   - Cobro de recargos: pasar a BLOQUEADO_POR_SALDO.
//   - Resolución de incidencias: BLOQUEADO_POR_INCIDENCIAS -> ACTIVO.
//   - Pago completo: ACTIVO después de ErrPendingSurcharge.
type UpdateStatus struct {
	repo ports.CustomerRepository
}

// NewUpdateStatus inyecta el repositorio.
func NewUpdateStatus(repo ports.CustomerRepository) *UpdateStatus {
	return &UpdateStatus{repo: repo}
}

// Run aplica el cambio de status. Devuelve domain.ErrInvalidStatus si el
// status no es válido y domain.ErrCustomerNotFound si el cliente no existe.
func (uc *UpdateStatus) Run(ctx context.Context, id uuid.UUID, status domain.CustomerStatus) (domain.Customer, error) {
	return uc.repo.UpdateStatus(ctx, id, status)
}
