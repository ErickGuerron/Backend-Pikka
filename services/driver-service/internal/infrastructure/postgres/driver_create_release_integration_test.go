//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
)

func TestCreateAndReleaseDriverAgainstPostgres(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	svc := driver.NewService(repo, func() time.Time { return t0 })

	created, err := svc.Create(ctx, domaintest.Input(driverB))
	require.NoError(t, err)

	_, err = svc.Create(ctx, domaintest.Input(driverB))
	assert.ErrorIs(t, err, ports.ErrDriverExists, "the unique index on user_id rejects a second driver for the same user")

	_, err = svc.ReserveDriver(ctx, created.ID, "route-1")
	require.NoError(t, err)
	_, err = svc.ReleaseDriver(ctx, created.ID, "route-1")
	require.NoError(t, err)

	stored, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, stored.Status)
	assert.Empty(t, stored.RouteID)
}
