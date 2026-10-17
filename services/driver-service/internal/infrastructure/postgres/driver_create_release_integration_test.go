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
)

func TestCreateAndReleaseDriverAgainstPostgres(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	svc := driver.NewService(repo, func() time.Time { return t0 })
	in := domain.NewDriverInput{UserID: userB, FullName: "Ana Ruiz", Phone: "0991234567"}

	created, err := svc.CreateDriver(ctx, in)
	require.NoError(t, err)

	_, err = svc.CreateDriver(ctx, in)
	assert.ErrorIs(t, err, ports.ErrDriverExists, "the unique index on user_id rejects a second driver for the same user")

	_, err = svc.CreateDriver(ctx, domain.NewDriverInput{UserID: "no-es-uuid", FullName: "Ana", Phone: "0991234567"})
	assert.ErrorIs(t, err, domain.ErrInvalidUserID, "a user_id that is not a UUID is a validation error")

	_, err = svc.ReserveDriver(ctx, created.ID, "route-1")
	require.NoError(t, err)
	_, err = svc.ReleaseDriver(ctx, created.ID, "route-1")
	require.NoError(t, err)

	stored, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, stored.Status)
	assert.Empty(t, stored.RouteID)
}
