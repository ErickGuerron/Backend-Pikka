package application

import (
	"context"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// SignupCustomerResult es la respuesta de un signup de cliente exitoso.
type SignupCustomerResult struct {
	AccessToken string
	ExpiresIn   time.Duration
	User        domain.User
}

// SignupCustomer registra un cliente nuevo (rol CUSTOMER) y emite un JWT.
// Es el único caso de uso público del Auth Service: no exige caller ADMIN
// porque el llamador es el Gateway en una ruta sin JWT.
type SignupCustomer struct {
	users  ports.UserRepository
	hasher ports.PasswordHasher
	tokens ports.TokenIssuer
}

// NewSignupCustomer construye el caso de uso.
func NewSignupCustomer(users ports.UserRepository, hasher ports.PasswordHasher, tokens ports.TokenIssuer) *SignupCustomer {
	return &SignupCustomer{users: users, hasher: hasher, tokens: tokens}
}

// Execute valida la entrada, hashea la contraseña, persiste el usuario y emite
// un token con rol CUSTOMER. Sigue las mismas reglas que CreateUser pero
// forzando Role = RoleCustomer y sin exigir caller.
func (uc *SignupCustomer) Execute(ctx context.Context, email, password, fullName string) (SignupCustomerResult, error) {
	in := domain.NewUserInput{
		Email:    email,
		Password: password,
		FullName: fullName,
		Role:     domain.RoleCustomer,
	}
	valid, err := in.Validate()
	if err != nil {
		return SignupCustomerResult{}, err
	}
	hash, err := uc.hasher.Hash(valid.Password)
	if err != nil {
		return SignupCustomerResult{}, err
	}
	user, err := uc.users.Create(ctx, domain.User{
		Email:        valid.Email,
		FullName:     valid.FullName,
		PasswordHash: hash,
		Role:         domain.RoleCustomer,
		Active:       true,
	})
	if err != nil {
		return SignupCustomerResult{}, err
	}
	token, ttl, err := uc.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return SignupCustomerResult{}, err
	}
	return SignupCustomerResult{AccessToken: token, ExpiresIn: ttl, User: user}, nil
}
