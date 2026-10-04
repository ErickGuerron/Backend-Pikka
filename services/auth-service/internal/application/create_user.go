package application

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// CreateUser registra un usuario. Solo un ADMIN puede hacerlo.
type CreateUser struct {
	users  ports.UserRepository
	hasher ports.PasswordHasher
}

// NewCreateUser construye el caso de uso.
func NewCreateUser(users ports.UserRepository, hasher ports.PasswordHasher) *CreateUser {
	return &CreateUser{users: users, hasher: hasher}
}

// Execute valida la entrada, verifica permisos y persiste el usuario.
func (uc *CreateUser) Execute(ctx context.Context, caller ports.Caller, in domain.NewUserInput) (domain.User, error) {
	if caller.Role != domain.RoleAdmin {
		return domain.User{}, domain.ErrForbidden
	}
	return uc.create(ctx, in)
}

func (uc *CreateUser) create(ctx context.Context, in domain.NewUserInput) (domain.User, error) {
	valid, err := in.Validate()
	if err != nil {
		return domain.User{}, err
	}
	hash, err := uc.hasher.Hash(valid.Password)
	if err != nil {
		return domain.User{}, err
	}
	return uc.users.Create(ctx, domain.User{
		Email:        valid.Email,
		FullName:     valid.FullName,
		PasswordHash: hash,
		Role:         valid.Role,
		Active:       true,
	})
}
