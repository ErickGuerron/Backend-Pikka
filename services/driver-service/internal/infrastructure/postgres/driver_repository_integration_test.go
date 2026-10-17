//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/postgres"
)

const (
	driverA = "11111111-1111-4111-8111-111111111111"
	driverB = "22222222-2222-4222-8222-222222222222"
	driverC = "33333333-3333-4333-8333-333333333333"
	userA   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

var t0 = time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)

// openRepo prepara la base de pruebas: aplica migraciones y vacía la tabla.
func openRepo(t *testing.T) (*postgres.DriverRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("DRIVER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DRIVER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	// El esquema drivers lo crea el script de inicialización; el usuario de servicio no puede crearlo.
	require.NoError(t, postgres.Migrate(dsn))
	_, err = admin.Exec(ctx, `TRUNCATE drivers`)
	require.NoError(t, err)
	return postgres.NewDriverRepository(admin), admin
}

func newDriver(t *testing.T, id string) *domain.Driver {
	t.Helper()
	d, err := domain.NewDriver(id, domaintest.Input(userA), t0)
	require.NoError(t, err)
	return d
}

func TestDriverRepositoryCreateGet(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	d := newDriver(t, driverA)
	require.NoError(t, repo.Create(ctx, d))

	got, err := repo.Get(ctx, driverA)

	require.NoError(t, err)
	assert.Equal(t, d.FullName, got.FullName)
	assert.Equal(t, domain.StatusAvailable, got.Status)
	assert.Equal(t, 1, got.Version)
	assert.True(t, got.Active)
}

func TestDriverRepositoryCreateDuplicado(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))

	err := repo.Create(ctx, newDriver(t, driverA))

	assert.ErrorIs(t, err, ports.ErrDriverExists)
}

func TestDriverRepositoryGetNoEncontrado(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, driverB)
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	_, err = repo.Get(ctx, "no-es-uuid")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestDriverRepositorySaveConflictoDeVersion(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))
	first, err := repo.Get(ctx, driverA)
	require.NoError(t, err)
	stale, err := repo.Get(ctx, driverA)
	require.NoError(t, err)

	require.NoError(t, first.Reserve("route-1", t0))
	require.NoError(t, repo.Save(ctx, first, 1))

	require.NoError(t, stale.Reserve("route-2", t0))
	err = repo.Save(ctx, stale, 1)

	assert.ErrorIs(t, err, ports.ErrVersionConflict)
	got, err := repo.Get(ctx, driverA)
	require.NoError(t, err)
	assert.Equal(t, "route-1", got.RouteID, "la escritura obsoleta no debe persistirse")
}

func TestDriverRepositorySaveNoEncontrado(t *testing.T) {
	repo, _ := openRepo(t)
	d := newDriver(t, driverB)

	err := repo.Save(context.Background(), d, 1)

	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestDriverRepositoryFindAvailable(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))
	require.NoError(t, repo.Create(ctx, newDriver(t, driverB)))
	require.NoError(t, repo.Create(ctx, newDriver(t, driverC)))

	inactive, err := repo.Get(ctx, driverB)
	require.NoError(t, err)
	inactive.Deactivate(t0)
	require.NoError(t, repo.Save(ctx, inactive, inactive.Version-1))

	assigned, err := repo.Get(ctx, driverC)
	require.NoError(t, err)
	require.NoError(t, assigned.Reserve("route-1", t0))
	require.NoError(t, repo.Save(ctx, assigned, assigned.Version-1))

	got, err := repo.FindAvailable(ctx, 10)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, driverA, got[0].ID)
}

// La reserva concurrente se prueba end-to-end: el servicio de aplicación con el repositorio
// de PostgreSQL. Veinte rutas compiten por el mismo repartidor y solo una gana.
func TestReserveDriverConcurrentePostgres(t *testing.T) {
	repo, _ := openRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newDriver(t, driverA)))
	svc := driver.NewService(repo, func() time.Time { return t0 })

	const contenders = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
		losers  int
		others  []error
	)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(routeID string) {
			defer wg.Done()
			_, err := svc.ReserveDriver(ctx, driverA, routeID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				winners++
			case errors.Is(err, domain.ErrDriverAlreadyReserved):
				losers++
			default:
				others = append(others, err)
			}
		}(fmt.Sprintf("route-%d", i))
	}
	wg.Wait()

	assert.Empty(t, others, "solo se esperan éxitos o ErrDriverAlreadyReserved")
	assert.Equal(t, 1, winners)
	assert.Equal(t, contenders-1, losers)
}
