package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// fakeRepo es un repositorio en memoria que cumple ports.ZoneRepository.
type fakeRepo struct {
	zones map[string]domain.Zone
}

func newFakeRepo() *fakeRepo { return &fakeRepo{zones: map[string]domain.Zone{}} }

func (r *fakeRepo) Save(_ context.Context, z domain.Zone) (domain.Zone, error) {
	for _, existing := range r.zones {
		if existing.Code == z.Code {
			return domain.Zone{}, domain.ErrZoneCodeTaken
		}
	}
	z.ID = uuid.NewString()
	r.zones[z.ID] = z
	return z, nil
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (domain.Zone, error) {
	z, ok := r.zones[id]
	if !ok {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	return z, nil
}

func (r *fakeRepo) FindContaining(_ context.Context, _ domain.Coordinate) (domain.Zone, error) {
	for _, z := range r.zones {
		return z, nil // el fake no calcula geometría; la prueba real está en persistence
	}
	return domain.Zone{}, domain.ErrZoneNotFound
}

func ring(t *testing.T) []domain.Coordinate {
	t.Helper()
	pts := [][2]float64{{-0.20, -78.50}, {-0.20, -78.40}, {-0.10, -78.40}, {-0.10, -78.50}, {-0.20, -78.50}}
	out := make([]domain.Coordinate, 0, len(pts))
	for _, p := range pts {
		c, err := domain.NewCoordinate(p[0], p[1])
		require.NoError(t, err)
		out = append(out, c)
	}
	return out
}

func TestCreateZone_PersistsValidZone(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())

	zone, err := svc.CreateZone(context.Background(), "NORTE", "Zona Norte", ring(t))

	require.NoError(t, err)
	assert.NotEmpty(t, zone.ID)
	assert.Equal(t, "NORTE", zone.Code)
}

func TestCreateZone_RejectsInvalidZoneBeforePersisting(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewZoneService(repo)

	_, err := svc.CreateZone(context.Background(), "", "Zona", ring(t))

	assert.ErrorIs(t, err, domain.ErrInvalidCode)
	assert.Empty(t, repo.zones)
}

func TestCreateZone_PropagatesDuplicateCode(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())
	_, err := svc.CreateZone(context.Background(), "NORTE", "Zona Norte", ring(t))
	require.NoError(t, err)

	_, err = svc.CreateZone(context.Background(), "NORTE", "Otra", ring(t))

	assert.True(t, errors.Is(err, domain.ErrZoneCodeTaken))
}

func TestGetZone_InvalidIDIsNotFound(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())

	_, err := svc.GetZone(context.Background(), "no-es-uuid")

	assert.ErrorIs(t, err, domain.ErrZoneNotFound)
}

func TestClassifyPoint_RejectsOutOfRangeCoordinate(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())

	_, err := svc.ClassifyPoint(context.Background(), 200, 0)

	assert.ErrorIs(t, err, domain.ErrInvalidCoordinate)
}
