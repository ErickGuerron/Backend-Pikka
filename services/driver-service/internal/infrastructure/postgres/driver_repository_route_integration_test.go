//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
)

func TestDriverRepositoryFindByRouteID(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))
	require.NoError(t, repo.Create(ctx, newDriver(t, driverB)))

	reserved, err := repo.Get(ctx, driverA)
	require.NoError(t, err)
	require.NoError(t, reserved.Reserve("route-1", t0))
	require.NoError(t, repo.Save(ctx, reserved, reserved.Version-1))

	got, err := repo.FindByRouteID(ctx, "route-1")
	require.NoError(t, err)
	assert.Equal(t, driverA, got.ID)
	assert.Equal(t, "route-1", got.RouteID)

	_, err = repo.FindByRouteID(ctx, "route-2")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound, "una ruta sin repartidor no debe devolver otro")

	_, err = repo.FindByRouteID(ctx, "")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound, "la cadena vacía no debe coincidir con repartidores libres")
}
