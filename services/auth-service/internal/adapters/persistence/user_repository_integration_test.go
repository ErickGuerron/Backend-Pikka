//go:build integration

package persistence_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/postgres"
)

// Requiere AUTH_TEST_DATABASE_URL apuntando a una base con el esquema auth.
// Ejecutar con: go test -tags integration ./...
func TestUserRepository(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	require.NoError(t, postgres.Migrate(dsn))
	pool, err := postgres.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `TRUNCATE users`)
	require.NoError(t, err)

	repo := persistence.NewUserRepository(pool)
	email := "it-" + uuid.NewString()[:8] + "@example.com"

	created, err := repo.Create(ctx, domain.User{Email: email, FullName: "IT", PasswordHash: "h", Role: domain.RoleDriver, Active: true})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, 1, created.Version)

	_, err = repo.Create(ctx, domain.User{Email: email, FullName: "IT", PasswordHash: "h", Role: domain.RoleDriver, Active: true})
	assert.ErrorIs(t, err, domain.ErrEmailAlreadyExists)

	byEmail, err := repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, created.ID, byEmail.ID)

	byID, err := repo.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleDriver, byID.Role)

	_, err = repo.FindByID(ctx, "not-a-uuid")
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
	_, err = repo.FindByID(ctx, uuid.NewString())
	assert.ErrorIs(t, err, domain.ErrUserNotFound)

	n, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, err = pool.Exec(ctx, `INSERT INTO users (email, full_name, password_hash, role) VALUES ('UPPER@example.com','x','h','DRIVER')`)
	assert.Error(t, err, "el CHECK debe rechazar correos con mayúsculas")
	_, err = pool.Exec(ctx, `INSERT INTO users (email, full_name, password_hash, role) VALUES ('r@example.com','x','h','ROOT')`)
	assert.Error(t, err, "el CHECK debe rechazar roles desconocidos")
}
