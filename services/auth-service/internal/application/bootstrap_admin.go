package application

import (
	"context"
	"errors"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// BootstrapAdmin crea el primer ADMIN cuando la tabla de usuarios está vacía.
// Permite operar un entorno nuevo sin insertar filas a mano.
type BootstrapAdmin struct {
	users  ports.UserRepository
	create *CreateUser
}

// NewBootstrapAdmin construye el caso de uso.
func NewBootstrapAdmin(users ports.UserRepository, hasher ports.PasswordHasher) *BootstrapAdmin {
	return &BootstrapAdmin{users: users, create: NewCreateUser(users, hasher)}
}

// Execute devuelve true si creó el administrador.
func (uc *BootstrapAdmin) Execute(ctx context.Context, email, password string) (bool, error) {
	n, err := uc.users.Count(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	_, err = uc.create.create(ctx, domain.NewUserInput{
		Email:    email,
		Password: password,
		FullName: "Administrador",
		Role:     domain.RoleAdmin,
	})
	// Otra instancia pudo crearlo al mismo tiempo; la restricción única lo resuelve.
	if errors.Is(err, domain.ErrEmailAlreadyExists) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
