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

	// newDriver usa el id del repartidor como user_id: cada repartidor necesita un usuario distinto.
	got, err := repo.FindByUserID(ctx, driverA)
	require.NoError(t, err)
	assert.Equal(t, driverA, got.ID)
	assert.Equal(t, driverA, got.UserID)

	_, err = repo.FindByUserID(ctx, driverC)
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	_, err = repo.FindByUserID(ctx, "no-es-uuid")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestDriverRepositoryListIncludesInactiveAndHonoursLimit(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	for _, id := range []string{driverC, driverA, driverB} {
		require.NoError(t, repo.Create(ctx, newDriver(t, id)))
	}
	inactive, err := repo.Get(ctx, driverB)
	require.NoError(t, err)
	inactive.Deactivate(t0)
	require.NoError(t, repo.Save(ctx, inactive, inactive.Version-1))

	all, err := repo.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, all, 3)
	// Mismo nombre y apellido: desempata el id.
	assert.Equal(t, []string{driverA, driverB, driverC}, []string{all[0].ID, all[1].ID, all[2].ID})
	assert.False(t, all[1].Active, "los inactivos también se listan")

	limited, err := repo.List(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}
