// Package application implementa los casos de uso del Auth Service.
package application

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// ListUsers devuelve todos los usuarios del sistema. Solo ADMIN (ADR 0008 §3):
// la comprobación del rol se hace antes de tocar la base, así un caller sin
// permiso ni siquiera provoca una consulta SQL.
type ListUsers struct {
	users ports.UserRepository
}

// NewListUsers construye el caso de uso.
func NewListUsers(users ports.UserRepository) *ListUsers {
	return &ListUsers{users: users}
}

// Execute valida el rol del caller y devuelve el listado. Cualquier error del
// repositorio se propaga sin envolver para que el adaptador gRPC lo mapee al
// código correspondiente.
func (uc *ListUsers) Execute(ctx context.Context, callerRole domain.Role) ([]domain.User, error) {
	if callerRole != domain.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	return uc.users.List(ctx)
}
