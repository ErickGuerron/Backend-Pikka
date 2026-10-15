package application

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// UpdateUser edita nombre y/o rol de un usuario. Solo ADMIN. Las reglas de
// negocio viven en el dominio; el repositorio las aplica dentro de la misma
// transacción que el UPDATE para que dos ediciones concurrentes no dejen el
// sistema sin ADMIN activo.
type UpdateUser struct {
	users ports.UserRepository
}

// NewUpdateUser construye el caso de uso.
func NewUpdateUser(users ports.UserRepository) *UpdateUser {
	return &UpdateUser{users: users}
}

// Execute valida el rol del caller y aplica la edición.
func (uc *UpdateUser) Execute(ctx context.Context, caller ports.Caller, id string, in domain.UserUpdate) (domain.User, error) {
	if caller.Role != domain.RoleAdmin {
		return domain.User{}, domain.ErrForbidden
	}
	return uc.users.UpdateUser(ctx, id, func(cur domain.User, activeAdmins int) (domain.User, error) {
		return domain.ApplyUpdate(cur, activeAdmins, in)
	})
}
