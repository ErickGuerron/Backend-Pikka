//go:build integration

package persistence_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/postgres"
)

// Requiere AUTH_TEST_DATABASE_URL. Ejecutar con: go test -tags integration ./...
func TestUserRepositoryUpdateUser(t *testing.T) {
	dsn := isolatedDSN(t, "AUTH_TEST_DATABASE_URL")
	ctx := context.Background()
	require.NoError(t, postgres.Migrate(dsn))
	pool, err := postgres.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	// La semilla trae un ADMIN activo (admin@pikka.com). Esta prueba necesita que el unico ADMIN activo sea
	// el que ella crea, asi que la semilla se desactiva dentro de su propio esquema.
	_, err = pool.Exec(ctx, `UPDATE users SET active = false WHERE id = '00000000-0000-4000-8000-000000000001'`)
	require.NoError(t, err)

	repo := persistence.NewUserRepository(pool)
	mk := func(role domain.Role) domain.User {
		u, err := repo.Create(ctx, domain.User{Email: "it-" + uuid.NewString()[:8] + "@example.com", FullName: "IT", PasswordHash: "h", Role: role, Active: true})
		require.NoError(t, err)
		return u
	}
	adminA := mk(domain.RoleAdmin)
	driver := mk(domain.RoleDriver)

	renamed := "Nombre Nuevo"
	got, err := repo.UpdateUser(ctx, driver.ID, func(cur domain.User, admins int) (domain.User, error) {
		return domain.ApplyUpdate(cur, admins, domain.UserUpdate{FullName: &renamed})
	})
	require.NoError(t, err)
	assert.Equal(t, renamed, got.FullName)
	assert.Equal(t, 2, got.Version, "cada edición incrementa version")

	_, err = repo.UpdateUser(ctx, uuid.NewString(), deactivate(adminA.ID))
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
	_, err = repo.UpdateUser(ctx, "not-a-uuid", deactivate(adminA.ID))
	assert.ErrorIs(t, err, domain.ErrUserNotFound)

	// Único ADMIN activo: la regla se evalúa con el conteo real de la base.
	_, err = repo.UpdateUser(ctx, adminA.ID, deactivate("otro"))
	assert.ErrorIs(t, err, domain.ErrLastActiveAdmin)

	// Con un segundo ADMIN activo sí se puede desactivar el primero.
	adminB := mk(domain.RoleAdmin)
	off, err := repo.UpdateUser(ctx, adminA.ID, deactivate(adminB.ID))
	require.NoError(t, err)
	assert.False(t, off.Active)
	assert.Equal(t, 2, off.Version)

	stored, err := repo.FindByID(ctx, adminA.ID)
	require.NoError(t, err)
	assert.False(t, stored.Active, "la baja queda persistida")
	assert.Equal(t, 2, stored.Version)
}

func deactivate(actorID string) ports.UserMutation {
	return func(cur domain.User, admins int) (domain.User, error) {
		return domain.ApplyDeactivation(cur, admins, actorID)
	}
}
