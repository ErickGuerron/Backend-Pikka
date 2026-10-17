package driver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestReleaseDriverFreesTheRoute(t *testing.T) {
	ctx := context.Background()
	repo, svc := seed(t, "d1")
	_, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)

	got, err := svc.ReleaseDriver(ctx, "d1", "route-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, got.Status)
	stored, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, stored.Status)
	assert.Empty(t, stored.RouteID)

	// The driver can take a new route after the release.
	_, err = svc.ReserveDriver(ctx, "d1", "route-2")
	require.NoError(t, err)
}

func TestReleaseDriverIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo, svc := seed(t, "d1")
	before, err := repo.Get(ctx, "d1")
	require.NoError(t, err)

	got, err := svc.ReleaseDriver(ctx, "d1", "route-1")

	require.NoError(t, err, "releasing a driver with no reservation succeeds")
	assert.Equal(t, domain.StatusAvailable, got.Status)
	assert.Equal(t, before.Version, got.Version, "no change, no version bump")
}

func TestReleaseDriverRefusesAnotherRoute(t *testing.T) {
	ctx := context.Background()
	repo, svc := seed(t, "d1")
	_, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)

	_, err = svc.ReleaseDriver(ctx, "d1", "route-2")

	assert.ErrorIs(t, err, domain.ErrRouteMismatch)
	stored, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAssigned, stored.Status, "the reservation of route-1 is untouched")
	assert.Equal(t, "route-1", stored.RouteID)
}

func TestReleaseDriverUnknownDriver(t *testing.T) {
	_, svc := seed(t, "d1")

	_, err := svc.ReleaseDriver(context.Background(), "missing", "route-1")

	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}
