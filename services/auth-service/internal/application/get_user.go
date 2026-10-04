package application

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// GetUser devuelve un usuario. Un usuario puede verse a sí mismo; ADMIN y
// OPERATOR pueden ver a cualquiera.
type GetUser struct {
	users ports.UserRepository
}

// NewGetUser construye el caso de uso.
func NewGetUser(users ports.UserRepository) *GetUser {
	return &GetUser{users: users}
}

// Execute busca el usuario aplicando la regla de autorización.
func (uc *GetUser) Execute(ctx context.Context, caller ports.Caller, id string) (domain.User, error) {
	if caller.UserID != id && caller.Role != domain.RoleAdmin && caller.Role != domain.RoleOperator {
		return domain.User{}, domain.ErrForbidden
	}
	return uc.users.FindByID(ctx, id)
}
