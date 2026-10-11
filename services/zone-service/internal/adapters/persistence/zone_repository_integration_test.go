//go:build integration

package persistence_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/infrastructure/postgres"
)

// Requiere ZONE_TEST_DATABASE_URL apuntando al PostgreSQL del compose con el
// usuario zone_service_user y search_path=zones,public (PostGIS vive en public).
// Sin la variable, se omite.
func setupRepo(t *testing.T) (*persistence.ZoneRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("ZONE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ZONE_TEST_DATABASE_URL no definida")
	}

	ctx := context.Background()
	require.NoError(t, postgres.Migrate(dsn))

	pool, err := postgres.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return persistence.NewZoneRepository(pool), pool
}

func coord(t *testing.T, lat, lng float64) domain.Coordinate {
	t.Helper()
	c, err := domain.NewCoordinate(lat, lng)
	require.NoError(t, err)
	return c
}

// squareZone crea una zona de prueba con código único y la borra al terminar.
func squareZone(t *testing.T, repo *persistence.ZoneRepository, pool *pgxpool.Pool, minLat, minLng, size float64) domain.Zone {
	t.Helper()
	ring := []domain.Coordinate{
		coord(t, minLat, minLng),
		coord(t, minLat, minLng+size),
		coord(t, minLat+size, minLng+size),
		coord(t, minLat+size, minLng),
		coord(t, minLat, minLng),
	}
	code := "IT-" + uuid.NewString()[:8]
	zone, err := domain.NewZone(code, "Zona de prueba", ring)
	require.NoError(t, err)

	saved, err := repo.Save(context.Background(), zone)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM zones WHERE id = $1`, saved.ID)
	})
	return saved
}

func TestZoneRepository_SaveAndFindByID_RoundTripsRing(t *testing.T) {
	repo, pool := setupRepo(t)
	saved := squareZone(t, repo, pool, -0.20, -78.50, 0.10)

	found, err := repo.FindByID(context.Background(), saved.ID)

	require.NoError(t, err)
	assert.Equal(t, saved.Code, found.Code)
	assert.Len(t, found.Ring, 5)
	assert.Equal(t, found.Ring[0], found.Ring[4], "el anillo debe seguir cerrado")
}

func TestZoneRepository_FindContaining_ClassifiesInsideAndOutside(t *testing.T) {
	repo, pool := setupRepo(t)
	zone := squareZone(t, repo, pool, -0.20, -78.50, 0.10)
	ctx := context.Background()

	inside, err := repo.FindContaining(ctx, coord(t, -0.15, -78.45))
	require.NoError(t, err)
	assert.Equal(t, zone.ID, inside.ID)

	_, err = repo.FindContaining(ctx, coord(t, 10, 10))
	assert.ErrorIs(t, err, domain.ErrZoneNotFound)
}

func TestZoneRepository_FindContaining_PicksSmallestZoneOnOverlap(t *testing.T) {
	repo, pool := setupRepo(t)
	big := squareZone(t, repo, pool, -0.30, -78.60, 0.30)
	small := squareZone(t, repo, pool, -0.20, -78.50, 0.10)

	got, err := repo.FindContaining(context.Background(), coord(t, -0.15, -78.45))

	require.NoError(t, err)
	assert.Equal(t, small.ID, got.ID)
	assert.NotEqual(t, big.ID, got.ID)
}

func TestZoneRepository_Save_RejectsDuplicateCode(t *testing.T) {
	repo, pool := setupRepo(t)
	saved := squareZone(t, repo, pool, -0.20, -78.50, 0.10)

	dup, err := domain.NewZone(saved.Code, "Otra", saved.Ring)
	require.NoError(t, err)
	_, err = repo.Save(context.Background(), dup)

	assert.ErrorIs(t, err, domain.ErrZoneCodeTaken)
}

// Una dona: el hueco se guarda, se lee de vuelta y un punto dentro del hueco no pertenece a la zona.
func TestZoneRepository_DonutRoundTripsAndExcludesHole(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()

	square := func(minLat, minLng, size float64) []domain.Coordinate {
		return []domain.Coordinate{
			coord(t, minLat, minLng),
			coord(t, minLat, minLng+size),
			coord(t, minLat+size, minLng+size),
			coord(t, minLat+size, minLng),
			coord(t, minLat, minLng),
		}
	}
	// Zona aislada (lejos de las demás pruebas): exterior de 2x2 con un hueco de 1x1 centrado.
	outer := square(40.0, -3.0, 2.0)
	hole := square(40.5, -2.5, 1.0)
	code := "IT-" + uuid.NewString()[:8]
	zone, err := domain.NewZoneWithHoles(code, "Dona de prueba", outer, [][]domain.Coordinate{hole})
	require.NoError(t, err)

	saved, err := repo.Save(ctx, zone)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM zones WHERE id = $1`, saved.ID)
	})

	found, err := repo.FindByID(ctx, saved.ID)
	require.NoError(t, err)
	require.Len(t, found.Holes, 1)
	assert.Len(t, found.Holes[0], 5)
	assert.Len(t, found.Ring, 5)

	// Centro del hueco: fuera de la zona.
	_, err = repo.FindContaining(ctx, coord(t, 41.0, -2.0))
	assert.ErrorIs(t, err, domain.ErrZoneNotFound)

	// Entre el exterior y el hueco: dentro de la zona.
	got, err := repo.FindContaining(ctx, coord(t, 40.2, -2.9))
	require.NoError(t, err)
	assert.Equal(t, saved.ID, got.ID)
}

func TestZoneRepository_ListAllIncludesHoles(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()
	outer := []domain.Coordinate{coord(t, 41, -3), coord(t, 41, -1), coord(t, 43, -1), coord(t, 43, -3), coord(t, 41, -3)}
	hole := []domain.Coordinate{coord(t, 41.5, -2.5), coord(t, 41.5, -1.5), coord(t, 42.5, -1.5), coord(t, 42.5, -2.5), coord(t, 41.5, -2.5)}
	zone, err := domain.NewZoneWithHoles("IT-"+uuid.NewString()[:8], "Dona listado", outer, [][]domain.Coordinate{hole})
	require.NoError(t, err)
	saved, err := repo.Save(ctx, zone)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM zones WHERE id = $1`, saved.ID) })

	zones, err := repo.ListAll(ctx)
	require.NoError(t, err)
	var found *domain.Zone
	for i := range zones {
		if zones[i].ID == saved.ID {
			found = &zones[i]
		}
	}
	require.NotNil(t, found, "la zona guardada debe aparecer en el listado")
	assert.Len(t, found.Ring, 5)
	require.Len(t, found.Holes, 1)
	assert.Len(t, found.Holes[0], 5)
}

func TestZoneRepository_UpdateReplacesGeometryAndKeepsID(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()
	saved := squareZone(t, repo, pool, -0.20, -78.50, 0.10)

	bigger := domain.Zone{
		ID:   saved.ID,
		Code: saved.Code,
		Name: "Zona ampliada",
		Ring: []domain.Coordinate{coord(t, -0.30, -78.60), coord(t, -0.30, -78.40), coord(t, -0.10, -78.40), coord(t, -0.10, -78.60), coord(t, -0.30, -78.60)},
	}
	updated, err := repo.Update(ctx, bigger)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, updated.ID)

	found, err := repo.FindByID(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "Zona ampliada", found.Name)
	assert.Equal(t, bigger.Ring[0], found.Ring[0])
}

func TestZoneRepository_UpdateErrors(t *testing.T) {
	repo, pool := setupRepo(t)
	ctx := context.Background()
	first := squareZone(t, repo, pool, -0.20, -78.50, 0.10)
	second := squareZone(t, repo, pool, -0.40, -78.70, 0.10)

	_, err := repo.Update(ctx, domain.Zone{ID: "11111111-1111-1111-1111-111111111111", Code: "X", Name: "X", Ring: first.Ring})
	assert.ErrorIs(t, err, domain.ErrZoneNotFound)

	_, err = repo.Update(ctx, domain.Zone{ID: second.ID, Code: first.Code, Name: "Choca", Ring: second.Ring})
	assert.ErrorIs(t, err, domain.ErrZoneCodeTaken)
}

func TestZoneRepository_SpatialIndexExists(t *testing.T) {
	_, pool := setupRepo(t)

	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE schemaname = 'zones' AND indexname = 'zones_area_gist'`,
	).Scan(&count)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
