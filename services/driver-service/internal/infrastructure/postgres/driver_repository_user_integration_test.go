//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
)

func TestDriverRepositoryFindByUserID(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))

	got, err := repo.FindByUserID(ctx, userA)
	require.NoError(t, err)
	assert.Equal(t, driverA, got.ID)
	assert.Equal(t, userA, got.UserID)

	_, err = repo.FindByUserID(ctx, driverC)
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	_, err = repo.FindByUserID(ctx, "no-es-uuid")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}
