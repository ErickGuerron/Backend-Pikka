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

func TestZoneRepository_SpatialIndexExists(t *testing.T) {
	_, pool := setupRepo(t)

	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE schemaname = 'zones' AND indexname = 'zones_area_gist'`,
	).Scan(&count)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
