package application

import (
	"context"
	"errors"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// LoginResult es la respuesta de un login exitoso.
type LoginResult struct {
	AccessToken string
	ExpiresIn   time.Duration
	User        domain.User
}

// Login valida credenciales y emite un token de acceso.
type Login struct {
	users  ports.UserRepository
	hasher ports.PasswordHasher
	tokens ports.TokenIssuer
	// dummyHash se compara cuando el usuario no existe para que el tiempo de
	// respuesta no revele qué correos están registrados.
	dummyHash string
}

// NewLogin construye el caso de uso.
func NewLogin(users ports.UserRepository, hasher ports.PasswordHasher, tokens ports.TokenIssuer) (*Login, error) {
	dummy, err := hasher.Hash("dummy-password-for-timing")
	if err != nil {
		return nil, err
	}
	return &Login{users: users, hasher: hasher, tokens: tokens, dummyHash: dummy}, nil
}

// Execute autentica al usuario.
func (uc *Login) Execute(ctx context.Context, email, password string) (LoginResult, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil || password == "" {
		return LoginResult{}, domain.ErrInvalidCredentials
	}

	user, err := uc.users.FindByEmail(ctx, normalized)
	if errors.Is(err, domain.ErrUserNotFound) {
		uc.hasher.Compare(uc.dummyHash, password)
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}

	if !uc.hasher.Compare(user.PasswordHash, password) {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if !user.Active {
		return LoginResult{}, domain.ErrUserInactive
	}

	token, ttl, err := uc.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: token, ExpiresIn: ttl, User: user}, nil
}
