package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

var admin = ports.Caller{UserID: "admin", Role: domain.RoleAdmin}

func seedUser(t *testing.T, repo *fakeRepo, email string, role domain.Role, active bool) domain.User {
	t.Helper()
	u, err := repo.Create(context.Background(), domain.User{Email: email, FullName: "X", PasswordHash: "hashed:password123", Role: role, Active: active})
	require.NoError(t, err)
	return u
}

func TestLogin(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	hasher := &fakeHasher{}
	uc, err := application.NewLogin(repo, hasher, fakeTokens{})
	require.NoError(t, err)

	active := seedUser(t, repo, "driver@example.com", domain.RoleDriver, true)
	seedUser(t, repo, "off@example.com", domain.RoleDriver, false)

	t.Run("credenciales válidas emiten token", func(t *testing.T) {
		res, err := uc.Execute(ctx, " DRIVER@example.com ", "password123")
		require.NoError(t, err)
		assert.Equal(t, "token:"+active.ID+":DRIVER", res.AccessToken)
		assert.Equal(t, active.ID, res.User.ID)
	})

	t.Run("contraseña incorrecta", func(t *testing.T) {
		_, err := uc.Execute(ctx, "driver@example.com", "wrong-pass")
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("usuario inexistente también compara hash", func(t *testing.T) {
		before := hasher.compares
		_, err := uc.Execute(ctx, "nadie@example.com", "password123")
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
		assert.Equal(t, before+1, hasher.compares)
	})

	t.Run("correo inválido no revela detalle", func(t *testing.T) {
		_, err := uc.Execute(ctx, "no-es-correo", "password123")
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("usuario inactivo", func(t *testing.T) {
		_, err := uc.Execute(ctx, "off@example.com", "password123")
		assert.ErrorIs(t, err, domain.ErrUserInactive)
	})
}

func TestCreateUser(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	uc := application.NewCreateUser(repo, &fakeHasher{})
	in := domain.NewUserInput{Email: "Op@Example.com", Password: "password123", FullName: "Op", Role: domain.RoleOperator}

	t.Run("solo ADMIN puede crear", func(t *testing.T) {
		_, err := uc.Execute(ctx, ports.Caller{UserID: "x", Role: domain.RoleOperator}, in)
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("crea con correo normalizado y contraseña hasheada", func(t *testing.T) {
		u, err := uc.Execute(ctx, admin, in)
		require.NoError(t, err)
		assert.Equal(t, "op@example.com", u.Email)
		assert.Equal(t, "hashed:password123", u.PasswordHash)
		assert.True(t, u.Active)
	})

	t.Run("correo duplicado", func(t *testing.T) {
		_, err := uc.Execute(ctx, admin, in)
		assert.ErrorIs(t, err, domain.ErrEmailAlreadyExists)
	})

	t.Run("entrada inválida", func(t *testing.T) {
		bad := in
		bad.Password = "123"
		_, err := uc.Execute(ctx, admin, bad)
		assert.ErrorIs(t, err, domain.ErrInvalidPassword)
	})
}

func TestGetUser(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	uc := application.NewGetUser(repo)
	driver := seedUser(t, repo, "d@example.com", domain.RoleDriver, true)
	other := seedUser(t, repo, "o@example.com", domain.RoleDriver, true)

	u, err := uc.Execute(ctx, ports.Caller{UserID: driver.ID, Role: domain.RoleDriver}, driver.ID)
	require.NoError(t, err)
	assert.Equal(t, driver.ID, u.ID)

	_, err = uc.Execute(ctx, ports.Caller{UserID: driver.ID, Role: domain.RoleDriver}, other.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	_, err = uc.Execute(ctx, ports.Caller{UserID: "op", Role: domain.RoleOperator}, other.ID)
	assert.NoError(t, err)

	_, err = uc.Execute(ctx, admin, "missing")
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	uc := application.NewBootstrapAdmin(repo, &fakeHasher{})

	created, err := uc.Execute(ctx, "admin@example.com", "admin-password")
	require.NoError(t, err)
	assert.True(t, created)

	created, err = uc.Execute(ctx, "admin@example.com", "admin-password")
	require.NoError(t, err)
	assert.False(t, created, "no debe crear otro admin si ya hay usuarios")
}

func TestValidateToken(t *testing.T) {
	uc := application.NewValidateToken(fakeTokens{})
	c, err := uc.Execute("token:u1:ADMIN")
	require.NoError(t, err)
	assert.Equal(t, ports.Claims{UserID: "u1", Role: domain.RoleAdmin}, c)

	_, err = uc.Execute("")
	assert.ErrorIs(t, err, domain.ErrInvalidToken)
}
