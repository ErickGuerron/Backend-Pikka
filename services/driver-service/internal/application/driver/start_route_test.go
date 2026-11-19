package driver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestStartRouteMovesTheReservedDriverOnRoute(t *testing.T) {
	ctx := context.Background()
	repo, svc := seed(t, "d1")
	_, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)

	got, err := svc.StartRoute(ctx, "d1", "route-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusOnRoute, got.Status)
	stored, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOnRoute, stored.Status)
}

func TestStartRouteIsIdempotentForTheSameRoute(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")
	_, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)
	_, err = svc.StartRoute(ctx, "d1", "route-1")
	require.NoError(t, err)

	got, err := svc.StartRoute(ctx, "d1", "route-1")

	require.NoError(t, err, "a second PICKED_UP on the same route must not fail")
	assert.Equal(t, domain.StatusOnRoute, got.Status)
}

func TestStartRouteRefusesAnotherRoute(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")
	_, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)

	_, err = svc.StartRoute(ctx, "d1", "route-2")

	assert.ErrorIs(t, err, domain.ErrRouteMismatch)
}

func TestStartRouteRequiresAReservation(t *testing.T) {
	_, svc := seed(t, "d1")

	_, err := svc.StartRoute(context.Background(), "d1", "route-1")

	assert.ErrorIs(t, err, domain.ErrInvalidTransition)
}
