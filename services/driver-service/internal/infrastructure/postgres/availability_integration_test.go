//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// Activar, desactivar y conectarse se prueban contra PostgreSQL real a través del servicio.
func TestAvailabilityFlowPostgres(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))
	svc := driver.NewService(repo, func() time.Time { return t0 })

	got, err := svc.SetActive(ctx, driverA, false)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOffline, got.Status)

	_, err = svc.GoOnline(ctx, driverA)
	assert.ErrorIs(t, err, domain.ErrDriverInactive, "un repartidor inactivo no puede conectarse")

	_, err = svc.SetActive(ctx, driverA, true)
	require.NoError(t, err)
	got, err = svc.GoOnline(ctx, driverA)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, got.Status)

	available, err := svc.FindAvailableDrivers(ctx, 0)
	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Equal(t, driverA, available[0].ID)
}
