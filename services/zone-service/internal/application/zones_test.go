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

func (r *fakeRepo) Update(_ context.Context, z domain.Zone) (domain.Zone, error) {
	if _, ok := r.zones[z.ID]; !ok {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	r.zones[z.ID] = z
	return z, nil
}

func (r *fakeRepo) ListAll(_ context.Context) ([]domain.Zone, error) {
	out := make([]domain.Zone, 0, len(r.zones))
	for _, z := range r.zones {
		out = append(out, z)
	}
	return out, nil
}

func (r *fakeRepo) SetStatus(_ context.Context, id, status, reason string) (domain.Zone, error) {
	z, ok := r.zones[id]
	if !ok {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	z.Status, z.StatusReason = status, reason
	r.zones[id] = z
	return z, nil
}

func (r *fakeRepo) Delete(_ context.Context, id, _ string) error {
	z, ok := r.zones[id]
	if !ok {
		return domain.ErrZoneNotFound
	}
	if z.Status != domain.StatusInactive {
		return domain.ErrZoneNotInactive
	}
	delete(r.zones, id)
	return nil
}

func (r *fakeRepo) NextCode(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (r *fakeRepo) FindContaining(_ context.Context, _ domain.Coordinate) (domain.Zone, error) {
	for _, z := range r.zones {
		return z, nil // el fake no calcula geometría; la prueba real está en persistence
	}
	return domain.Zone{}, domain.ErrZoneNotFound
}

// validSettings es una configuración operativa válida para los casos de uso.
func validSettings() domain.Settings {
	return domain.Settings{
		DepotName:          "Hub Norte",
		DepotLatitude:      -0.179,
		DepotLongitude:     -78.483,
		MaxOrdersPerRoute:  4,
		RouteCapacityKg:    45,
		ServiceDays:        []string{"MON"},
		DeliveryWindowFrom: "08:00",
		DeliveryWindowTo:   "20:00",
		CutoffTime:         "14:00",
		DispatchTime:       "15:30",
		DailyOrderLimit:    250,
	}
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

	zone, err := svc.CreateZoneWithHoles(context.Background(), "NORTE", "Zona Norte", ring(t), nil, validSettings())

	require.NoError(t, err)
	assert.NotEmpty(t, zone.ID)
	assert.Equal(t, "NORTE", zone.Code)
}

func TestCreateZone_GeneratesCodeWhenEmpty(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())

	zone, err := svc.CreateZoneWithHoles(context.Background(), "", "Zona Norte", ring(t), nil, validSettings())

	require.NoError(t, err)
	assert.Equal(t, "NORTE-001", zone.Code)
	assert.Equal(t, domain.StatusActive, zone.Status)
}

func TestCreateZone_RejectsInvalidNameBeforePersisting(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewZoneService(repo)

	_, err := svc.CreateZoneWithHoles(context.Background(), "", "   ", ring(t), nil, validSettings())

	assert.ErrorIs(t, err, domain.ErrInvalidName)
	assert.Empty(t, repo.zones)
}

func TestCreateZone_PropagatesDuplicateCode(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())
	_, err := svc.CreateZoneWithHoles(context.Background(), "NORTE", "Zona Norte", ring(t), nil, validSettings())
	require.NoError(t, err)

	_, err = svc.CreateZoneWithHoles(context.Background(), "NORTE", "Otra", ring(t), nil, validSettings())

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
