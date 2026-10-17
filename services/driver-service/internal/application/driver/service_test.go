package driver_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

var t0 = time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)

func fixedClock() func() time.Time {
	return func() time.Time { return t0 }
}

// seed crea un repartidor AVAILABLE y activo en un repositorio nuevo.
func seed(t *testing.T, id string) (*memory.DriverRepository, *driver.Service) {
	t.Helper()
	repo := memory.NewDriverRepository()
	d, err := domain.NewDriver(id, domaintest.Input("user-"+id), t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), d))
	return repo, driver.NewService(repo, fixedClock())
}

func TestSetActiveDesactivaYPersiste(t *testing.T) {
	ctx := context.Background()
	repo, svc := seed(t, "d1")

	got, err := svc.SetActive(ctx, "d1", false)

	require.NoError(t, err)
	assert.False(t, got.Active)
	assert.Equal(t, domain.StatusOffline, got.Status)
	stored, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.False(t, stored.Active)
	assert.Equal(t, domain.StatusOffline, stored.Status)
}

func TestGoOnlineTrasReactivar(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")
	_, err := svc.SetActive(ctx, "d1", false)
	require.NoError(t, err)
	_, err = svc.SetActive(ctx, "d1", true)
	require.NoError(t, err)

	got, err := svc.GoOnline(ctx, "d1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusAvailable, got.Status)
}

func TestGoOnlineSobreActivoFallaConInactivo(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")
	_, err := svc.SetActive(ctx, "d1", false)
	require.NoError(t, err)

	_, err = svc.GoOnline(ctx, "d1")

	assert.ErrorIs(t, err, domain.ErrDriverInactive)
}

func TestReserveDriverPasaAsignadoEIdempotente(t *testing.T) {
	ctx := context.Background()
	repo, svc := seed(t, "d1")

	first, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAssigned, first.Status)
	assert.Equal(t, "route-1", first.RouteID)

	again, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)
	assert.Equal(t, first.Version, again.Version, "repetir la misma ruta no sube la versión")

	stored, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, "route-1", stored.RouteID)
}

func TestReserveDriverOtraRutaFalla(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")
	_, err := svc.ReserveDriver(ctx, "d1", "route-1")
	require.NoError(t, err)

	_, err = svc.ReserveDriver(ctx, "d1", "route-2")

	assert.ErrorIs(t, err, domain.ErrDriverAlreadyReserved)
}

func TestReserveDriverInactivoFalla(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")
	_, err := svc.SetActive(ctx, "d1", false)
	require.NoError(t, err)

	_, err = svc.ReserveDriver(ctx, "d1", "route-1")

	assert.ErrorIs(t, err, domain.ErrDriverInactive)
}

func TestReserveDriverConcurrenteSoloUnoGana(t *testing.T) {
	ctx := context.Background()
	_, svc := seed(t, "d1")

	const contenders = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
		losers  int
	)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(routeID string) {
			defer wg.Done()
			_, err := svc.ReserveDriver(ctx, "d1", routeID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				winners++
			case errors.Is(err, domain.ErrDriverAlreadyReserved):
				losers++
			default:
				t.Errorf("error inesperado: %v", err)
			}
		}(fmt.Sprintf("route-%d", i))
	}
	wg.Wait()

	assert.Equal(t, 1, winners, "exactamente una ruta debe reservar al repartidor")
	assert.Equal(t, contenders-1, losers)
}

func TestFindAvailableDriversFiltraYOrdena(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	for _, id := range []string{"d3", "d1", "d2"} {
		d, err := domain.NewDriver(id, domaintest.Input("u-"+id), t0)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, d))
	}
	svc := driver.NewService(repo, fixedClock())
	_, err := svc.SetActive(ctx, "d2", false)
	require.NoError(t, err)
	_, err = svc.ReserveDriver(ctx, "d3", "route-1")
	require.NoError(t, err)

	got, err := svc.FindAvailableDrivers(ctx, 0)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "d1", got[0].ID)
}

func TestFindAvailableDriversLimite(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	for _, id := range []string{"d1", "d2", "d3"} {
		d, err := domain.NewDriver(id, domaintest.Input("u-"+id), t0)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, d))
	}
	svc := driver.NewService(repo, fixedClock())

	got, err := svc.FindAvailableDrivers(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	_, err = svc.FindAvailableDrivers(ctx, -1)
	assert.ErrorIs(t, err, driver.ErrInvalidLimit)

	// Mayor que el máximo no es error: el contrato lo recorta a 100.
	got, err = svc.FindAvailableDrivers(ctx, 500)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestRepositorioDevuelveNoEncontrado(t *testing.T) {
	_, svc := seed(t, "d1")

	_, err := svc.GoOnline(context.Background(), "missing")

	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestCreateGuardaElPerfilCompleto(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, fixedClock())

	got, err := svc.Create(ctx, domaintest.Input("user-1"))

	require.NoError(t, err)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "Ana Ruiz", got.FullName)
	assert.Equal(t, domain.VehicleMotorbike, got.VehicleType)
	assert.True(t, got.Active)
	assert.Equal(t, domain.StatusAvailable, got.Status)
	stored, err := repo.Get(ctx, got.ID)
	require.NoError(t, err)
	assert.Equal(t, got.Plate, stored.Plate)
}

func TestCreateRechazaDatosInvalidosYDuplicados(t *testing.T) {
	ctx := context.Background()
	svc := driver.NewService(memory.NewDriverRepository(), fixedClock())
	_, err := svc.Create(ctx, domaintest.Input("user-1"))
	require.NoError(t, err)

	bad := domaintest.Input("user-2")
	bad.VehicleType = "BICICLETA"
	_, err = svc.Create(ctx, bad)
	assert.ErrorIs(t, err, domain.ErrInvalidVehicleType)

	dupUser := domaintest.Input("user-1")
	dupUser.Identifier, dupUser.Plate = "OTRO12345", "OTRA123"
	_, err = svc.Create(ctx, dupUser)
	assert.ErrorIs(t, err, ports.ErrDriverExists, "mismo usuario")

	dupPlate := domaintest.Input("user-3")
	dupPlate.Plate = domaintest.Input("user-1").Plate
	_, err = svc.Create(ctx, dupPlate)
	assert.ErrorIs(t, err, ports.ErrDriverExists, "misma placa")

	dupID := domaintest.Input("user-4")
	dupID.Identifier = domaintest.Input("user-1").Identifier
	_, err = svc.Create(ctx, dupID)
	assert.ErrorIs(t, err, ports.ErrDriverExists, "mismo identificador")
}

func TestListDevuelveTodosOrdenadosPorApellidoYNombre(t *testing.T) {
	ctx := context.Background()
	svc := driver.NewService(memory.NewDriverRepository(), fixedClock())
	for _, n := range []struct{ user, first, last string }{
		{"user-1", "Zoe", "Vera"}, {"user-2", "Ana", "Vera"}, {"user-3", "Luis", "Andrade"},
	} {
		in := domaintest.Input(n.user)
		in.FirstName, in.LastName = n.first, n.last
		_, err := svc.Create(ctx, in)
		require.NoError(t, err)
	}
	all, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []string{"Luis Andrade", "Ana Vera", "Zoe Vera"}, []string{all[0].FullName, all[1].FullName, all[2].FullName})

	_, err = svc.SetActive(ctx, all[0].ID, false)
	require.NoError(t, err)
	again, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, again, 3, "los inactivos también aparecen en la lista")
	assert.False(t, again[0].Active)
}
