package driver_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
)

// conflictRepo rechaza siempre el guardado con conflicto de versión y cuenta los intentos.
type conflictRepo struct {
	ports.DriverRepository
	saves int
}

func (r *conflictRepo) Save(context.Context, *domain.Driver, int) error {
	r.saves++
	return ports.ErrVersionConflict
}

// missingSaveRepo falla el guardado con un error ajeno al conflicto.
type missingSaveRepo struct {
	ports.DriverRepository
}

func (missingSaveRepo) Save(context.Context, *domain.Driver, int) error {
	return ports.ErrDriverNotFound
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newServiceWithDriver(t *testing.T, id string) (*driver.Service, *memory.DriverRepository) {
	t.Helper()
	repo := memory.NewDriverRepository()
	d, err := domain.NewDriver(id, domaintest.Input("user-"+id), serviceNow)
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), d))
	return driver.NewService(repo, func() time.Time { return serviceNow }), repo
}

var serviceNow = time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)

func TestCreateAssignsUUIDv4AndRejectsDuplicates(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, func() time.Time { return serviceNow })

	d, err := svc.Create(ctx, domaintest.Input("user-new"))

	require.NoError(t, err)
	assert.Regexp(t, uuidPattern, d.ID)

	_, err = svc.Create(ctx, domaintest.Input("user-new"))
	assert.ErrorIs(t, err, ports.ErrDriverExists)
}

func TestCreatePropagatesDomainValidationBeforeStoring(t *testing.T) {
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, func() time.Time { return serviceNow })
	in := domaintest.Input("user-x")
	in.FirstName = ""

	_, err := svc.Create(context.Background(), in)

	assert.Error(t, err)
	_, getErr := repo.FindByUserID(context.Background(), "user-x")
	assert.ErrorIs(t, getErr, ports.ErrDriverNotFound, "un alta inválida no se guarda")
}

func TestGoOfflineAndGoOnlineChangeAvailability(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithDriver(t, "d1")

	off, err := svc.GoOffline(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOffline, off.Status)

	on, err := svc.GoOnline(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, on.Status)
}

func TestSetActiveTogglesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithDriver(t, "d1")

	off, err := svc.SetActive(ctx, "d1", false)
	require.NoError(t, err)
	assert.False(t, off.Active)

	again, err := svc.SetActive(ctx, "d1", false)
	require.NoError(t, err)
	assert.False(t, again.Active, "desactivar dos veces es idempotente")

	on, err := svc.SetActive(ctx, "d1", true)
	require.NoError(t, err)
	assert.True(t, on.Active)
}

func TestMutationsReportUnknownDrivers(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithDriver(t, "d1")

	_, err := svc.GoOffline(ctx, "nadie")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	_, err = svc.SetActive(ctx, "nadie", true)
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	_, err = svc.GoOnline(ctx, "nadie")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestMutationGivesUpAfterRepeatedVersionConflicts(t *testing.T) {
	ctx := context.Background()
	base := memory.NewDriverRepository()
	d, err := domain.NewDriver("d1", domaintest.Input("user-d1"), serviceNow)
	require.NoError(t, err)
	require.NoError(t, base.Create(ctx, d))
	repo := &conflictRepo{DriverRepository: base}
	svc := driver.NewService(repo, func() time.Time { return serviceNow })

	_, err = svc.GoOffline(ctx, "d1")

	assert.ErrorIs(t, err, ports.ErrVersionConflict)
	assert.Equal(t, 5, repo.saves, "se reintenta hasta cinco veces y luego se rinde")
}

func TestMutationStopsOnNonConflictSaveError(t *testing.T) {
	ctx := context.Background()
	base := memory.NewDriverRepository()
	d, err := domain.NewDriver("d1", domaintest.Input("user-d1"), serviceNow)
	require.NoError(t, err)
	require.NoError(t, base.Create(ctx, d))
	svc := driver.NewService(missingSaveRepo{DriverRepository: base}, func() time.Time { return serviceNow })

	_, err = svc.GoOffline(ctx, "d1")

	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestFindByRouteIDTrimsAndRejectsBlank(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, func() time.Time { return serviceNow })
	d, err := domain.NewDriver("d1", domaintest.Input("user-d1"), serviceNow)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, d))
	_, err = svc.ReserveDriver(ctx, "d1", "route-7")
	require.NoError(t, err)

	got, err := svc.FindByRouteID(ctx, "  route-7  ")
	require.NoError(t, err)
	assert.Equal(t, "d1", got.ID)

	_, err = svc.FindByRouteID(ctx, "   ")
	assert.ErrorIs(t, err, domain.ErrInvalidRouteID)
}

func TestFindByUserIDAndListDelegateToRepository(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithDriver(t, "d1")

	got, err := svc.FindByUserID(ctx, "user-d1")
	require.NoError(t, err)
	assert.Equal(t, "d1", got.ID)

	_, err = svc.FindByUserID(ctx, "nobody")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	list, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
