// Package ports define las interfaces que la capa de aplicación necesita del exterior.
package ports

import (
	"context"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// UserRepository persiste usuarios en el esquema auth.
type UserRepository interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	FindByID(ctx context.Context, id string) (domain.User, error)
	Count(ctx context.Context) (int, error)
}

// PasswordHasher calcula y verifica hashes de contraseñas.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) bool
}

// TokenIssuer emite y verifica tokens de acceso.
type TokenIssuer interface {
	Issue(userID string, role domain.Role) (token string, expiresIn time.Duration, err error)
	Verify(token string) (Claims, error)
}

// Claims es la identidad mínima contenida en un token.
type Claims struct {
	UserID string
	Role   domain.Role
}

// Caller es la identidad de quien invoca un caso de uso, propagada por el Gateway.
type Caller struct {
	UserID string
	Role   domain.Role
}
