package driver_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestCreateDriverRegistersAvailableActiveDriver(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, fixedClock())

	got, err := svc.CreateDriver(ctx, domain.NewDriverInput{UserID: " user-9 ", FullName: " Ana Ruiz ", Phone: "+593 99-123 4567"})

	require.NoError(t, err)
	assert.Regexp(t, uuidPattern, got.ID, "the service generates a UUID v4 id")
	assert.Equal(t, "user-9", got.UserID)
	assert.Equal(t, "Ana Ruiz", got.FullName)
	assert.Equal(t, "+593991234567", got.Phone, "phone is stored as the domain normalizes it")
	assert.True(t, got.Active)
	assert.Equal(t, domain.StatusAvailable, got.Status)
	assert.Equal(t, t0, got.CreatedAt)

	stored, err := repo.Get(ctx, got.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, stored.Status)
}

func TestCreateDriverRejectsInvalidInputWithoutStoring(t *testing.T) {
	tests := []struct {
		name  string
		in    domain.NewDriverInput
		isErr error
	}{
		{"blank user", domain.NewDriverInput{UserID: " ", FullName: "Ana", Phone: "0991234567"}, domain.ErrInvalidUserID},
		{"blank name", domain.NewDriverInput{UserID: "u", FullName: "  ", Phone: "0991234567"}, domain.ErrInvalidFullName},
		{"bad phone", domain.NewDriverInput{UserID: "u", FullName: "Ana", Phone: "abc"}, domain.ErrInvalidPhone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := memory.NewDriverRepository()
			svc := driver.NewService(repo, fixedClock())

			_, err := svc.CreateDriver(ctx, tt.in)

			assert.ErrorIs(t, err, tt.isErr)
			list, err := repo.FindAvailable(ctx, 10)
			require.NoError(t, err)
			assert.Empty(t, list, "a rejected create stores nothing")
		})
	}
}

func TestCreateDriverRejectsDuplicateUser(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, fixedClock())
	in := domain.NewDriverInput{UserID: "user-9", FullName: "Ana", Phone: "0991234567"}
	_, err := svc.CreateDriver(ctx, in)
	require.NoError(t, err)

	_, err = svc.CreateDriver(ctx, in)

	assert.ErrorIs(t, err, ports.ErrDriverExists)
}

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
