package application

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// DeactivateUser hace borrado lógico de un usuario. Solo ADMIN. Ver UpdateUser
// sobre por qué las reglas se evalúan dentro del repositorio.
type DeactivateUser struct {
	users ports.UserRepository
}

// NewDeactivateUser construye el caso de uso.
func NewDeactivateUser(users ports.UserRepository) *DeactivateUser {
	return &DeactivateUser{users: users}
}

// Execute desactiva al usuario indicado. El actor no puede desactivarse a sí mismo.
func (uc *DeactivateUser) Execute(ctx context.Context, caller ports.Caller, id string) (domain.User, error) {
	if caller.Role != domain.RoleAdmin {
		return domain.User{}, domain.ErrForbidden
	}
	return uc.users.UpdateUser(ctx, id, func(cur domain.User, activeAdmins int) (domain.User, error) {
		return domain.ApplyDeactivation(cur, activeAdmins, caller.UserID)
	})
}
