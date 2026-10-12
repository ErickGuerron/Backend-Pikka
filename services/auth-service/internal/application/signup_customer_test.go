package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// TestSignupCustomer agrupa los escenarios del caso de uso SignupCustomer.
// El caso de uso no existe todavía, por eso todos los sub-tests fallan al
// resolver application.NewSignupCustomer (RED del TDD).
func TestSignupCustomer(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	hasher := &fakeHasher{}
	uc := application.NewSignupCustomer(repo, hasher, fakeTokens{})

	t.Run("correo duplicado devuelve 409", func(t *testing.T) {
		_, err := uc.Execute(ctx, "cliente@example.com", "password123", "Cliente Uno")
		require.NoError(t, err)

		_, err = uc.Execute(ctx, " Cliente@Example.com ", "password123", "Cliente Dos")
		assert.ErrorIs(t, err, domain.ErrEmailAlreadyExists)
	})

	t.Run("correo inválido devuelve 400", func(t *testing.T) {
		_, err := uc.Execute(ctx, "no-es-correo", "password123", "Cliente")
		assert.ErrorIs(t, err, domain.ErrInvalidEmail)
	})

	t.Run("contraseña corta devuelve 400", func(t *testing.T) {
		_, err := uc.Execute(ctx, "nuevo@example.com", "corta", "Cliente")
		assert.ErrorIs(t, err, domain.ErrInvalidPassword)
	})

	t.Run("registro válido emite token", func(t *testing.T) {
		res, err := uc.Execute(ctx, "nuevo@example.com", "password123", "Cliente Nuevo")
		require.NoError(t, err)
		assert.NotEmpty(t, res.AccessToken)
		assert.Equal(t, domain.RoleCustomer, res.User.Role)
		assert.True(t, res.User.Active)
		assert.Equal(t, "nuevo@example.com", res.User.Email)
		assert.Equal(t, "Cliente Nuevo", res.User.FullName)
	})

	t.Run("el token contiene el rol CUSTOMER", func(t *testing.T) {
		res, err := uc.Execute(ctx, "otro@example.com", "password123", "Otro Cliente")
		require.NoError(t, err)
		claims, err := fakeTokens{}.Verify(res.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, domain.RoleCustomer, claims.Role)
	})
}
