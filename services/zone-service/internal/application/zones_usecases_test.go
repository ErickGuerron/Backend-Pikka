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

// failingRepo envuelve el repositorio en memoria y falla las operaciones cuyo error se configura.
type failingRepo struct {
	*fakeRepo
	saveErr, nextErr, updateErr, listErr, findErr, setStatusErr, deleteErr, containingErr error
}

func (r *failingRepo) Save(ctx context.Context, z domain.Zone) (domain.Zone, error) {
	if r.saveErr != nil {
		return domain.Zone{}, r.saveErr
	}
	return r.fakeRepo.Save(ctx, z)
}

func (r *failingRepo) NextCode(ctx context.Context, prefix string) (int, error) {
	if r.nextErr != nil {
		return 0, r.nextErr
	}
	return r.fakeRepo.NextCode(ctx, prefix)
}

func (r *failingRepo) Update(ctx context.Context, z domain.Zone) (domain.Zone, error) {
	if r.updateErr != nil {
		return domain.Zone{}, r.updateErr
	}
	return r.fakeRepo.Update(ctx, z)
}

func (r *failingRepo) ListAll(ctx context.Context) ([]domain.Zone, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.fakeRepo.ListAll(ctx)
}

func (r *failingRepo) FindByID(ctx context.Context, id string) (domain.Zone, error) {
	if r.findErr != nil {
		return domain.Zone{}, r.findErr
	}
	return r.fakeRepo.FindByID(ctx, id)
}

func (r *failingRepo) SetStatus(ctx context.Context, id, status, reason string) (domain.Zone, error) {
	if r.setStatusErr != nil {
		return domain.Zone{}, r.setStatusErr
	}
	return r.fakeRepo.SetStatus(ctx, id, status, reason)
}

func (r *failingRepo) Delete(ctx context.Context, id, reason string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.fakeRepo.Delete(ctx, id, reason)
}

func (r *failingRepo) FindContaining(ctx context.Context, p domain.Coordinate) (domain.Zone, error) {
	if r.containingErr != nil {
		return domain.Zone{}, r.containingErr
	}
	return r.fakeRepo.FindContaining(ctx, p)
}

var errBoom = errors.New("boom")

// seedZone guarda una zona activa en el repositorio y devuelve su identificador.
func seedZone(t *testing.T, repo *fakeRepo, code string) domain.Zone {
	t.Helper()
	z, err := domain.NewZoneWithHoles(code, "Zona Norte", ring(t), nil, validSettings())
	require.NoError(t, err)
	saved, err := repo.Save(context.Background(), z)
	require.NoError(t, err)
	return saved
}

func TestCreateZone_GeneratedCodeFailsWhenAllAttemptsTaken(t *testing.T) {
	repo := newFakeRepo()
	seedZone(t, repo, "NORTE-001") // NextCode del fake siempre devuelve 1
	svc := application.NewZoneService(repo)

	_, err := svc.CreateZoneWithHoles(context.Background(), "", "Zona Norte", ring(t), nil, validSettings())

	assert.ErrorIs(t, err, domain.ErrZoneCodeTaken)
	assert.Len(t, repo.zones, 1, "ningún intento debe persistir una zona nueva")
}

func TestCreateZone_NextCodeErrorIsWrapped(t *testing.T) {
	svc := application.NewZoneService(&failingRepo{fakeRepo: newFakeRepo(), nextErr: errBoom})

	_, err := svc.CreateZoneWithHoles(context.Background(), "", "Zona Norte", ring(t), nil, validSettings())

	assert.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "next zone code")
}

func TestCreateZone_SaveErrorIsWrapped(t *testing.T) {
	svc := application.NewZoneService(&failingRepo{fakeRepo: newFakeRepo(), saveErr: errBoom})

	_, err := svc.CreateZoneWithHoles(context.Background(), "NORTE", "Zona Norte", ring(t), nil, validSettings())

	assert.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "create zone")
}

func TestCreateZone_GeneratedCodeSaveErrorIsWrapped(t *testing.T) {
	svc := application.NewZoneService(&failingRepo{fakeRepo: newFakeRepo(), saveErr: errBoom})

	_, err := svc.CreateZoneWithHoles(context.Background(), "", "Zona Norte", ring(t), nil, validSettings())

	assert.ErrorIs(t, err, errBoom)
}

func TestCreateZone_RejectsInvalidGeometryAndSettings(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		ring     func(*testing.T) []domain.Coordinate
		settings domain.Settings
		wantErr  error
	}{
		{
			name: "open ring",
			code: "NORTE",
			ring: func(t *testing.T) []domain.Coordinate {
				r := ring(t)
				r[len(r)-1] = r[2] // el último punto ya no coincide con el primero
				return r
			},
			settings: validSettings(),
			wantErr:  domain.ErrRingNotClosed,
		},
		{
			name:     "invalid settings",
			code:     "NORTE",
			ring:     ring,
			settings: func() domain.Settings { s := validSettings(); s.DailyOrderLimit = 0; return s }(),
			wantErr:  domain.ErrInvalidSettings,
		},
		{
			name:     "code too long with explicit code",
			code:     string(make([]byte, 40)),
			ring:     ring,
			settings: validSettings(),
			wantErr:  domain.ErrInvalidCode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := application.NewZoneService(repo)

			_, err := svc.CreateZoneWithHoles(context.Background(), tt.code, "Zona Norte", tt.ring(t), nil, tt.settings)

			assert.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, repo.zones)
		})
	}
}

func TestCreateZone_RejectsInvalidHoleGeometry(t *testing.T) {
	outer := closedRing(t, [2]float64{0, 0}, [2]float64{0, 4}, [2]float64{4, 4}, [2]float64{4, 0})
	outside := closedRing(t, [2]float64{5, 5}, [2]float64{5, 7}, [2]float64{7, 7}, [2]float64{7, 5})
	svc := application.NewZoneService(newFakeRepo())

	_, err := svc.CreateZoneWithHoles(context.Background(), "DONA", "Zona Dona", outer, [][]domain.Coordinate{outside}, validSettings())

	assert.ErrorIs(t, err, domain.ErrHoleOutside)
}

func TestCreateZone_AcceptsDonutWithHole(t *testing.T) {
	outer := closedRing(t, [2]float64{0, 0}, [2]float64{0, 4}, [2]float64{4, 4}, [2]float64{4, 0})
	hole := closedRing(t, [2]float64{1, 1}, [2]float64{1, 3}, [2]float64{3, 3}, [2]float64{3, 1})
	svc := application.NewZoneService(newFakeRepo())

	zone, err := svc.CreateZoneWithHoles(context.Background(), "DONA", "Zona Dona", outer, [][]domain.Coordinate{hole}, validSettings())

	require.NoError(t, err)
	assert.Len(t, zone.Holes, 1)
	assert.Equal(t, domain.StatusActive, zone.Status)
}

// closedRing construye un anillo cerrado a partir de pares {lat, lng} para los casos de uso.
func closedRing(t *testing.T, pts ...[2]float64) []domain.Coordinate {
	t.Helper()
	all := append(append([][2]float64{}, pts...), pts[0])
	out := make([]domain.Coordinate, 0, len(all))
	for _, p := range all {
		c, err := domain.NewCoordinate(p[0], p[1])
		require.NoError(t, err)
		out = append(out, c)
	}
	return out
}

func TestUpdateZone_RedrawsActiveZoneKeepingCodeAndID(t *testing.T) {
	repo := newFakeRepo()
	existing := seedZone(t, repo, "NORTE")
	svc := application.NewZoneService(repo)

	updated, err := svc.UpdateZone(context.Background(), existing.ID, "Zona Norte Nueva", ring(t), nil, validSettings(), existing.Version)

	require.NoError(t, err)
	assert.Equal(t, existing.ID, updated.ID)
	assert.Equal(t, "NORTE", updated.Code)
	assert.Equal(t, "Zona Norte Nueva", updated.Name)
	assert.Equal(t, existing.Version, updated.Version)
}

func TestUpdateZone_ZeroExpectedVersionSkipsConflictCheck(t *testing.T) {
	repo := newFakeRepo()
	existing := seedZone(t, repo, "NORTE")
	svc := application.NewZoneService(repo)

	_, err := svc.UpdateZone(context.Background(), existing.ID, "Zona Norte", ring(t), nil, validSettings(), 0)

	assert.NoError(t, err)
}

func TestUpdateZone_ErrorCases(t *testing.T) {
	seeded := func(t *testing.T) (*fakeRepo, domain.Zone) {
		repo := newFakeRepo()
		return repo, seedZone(t, repo, "NORTE")
	}
	tests := []struct {
		name    string
		setup   func(t *testing.T) (*failingRepo, string, int, domain.Settings, string)
		wantErr error
	}{
		{
			name: "invalid identifier is not found",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				return &failingRepo{fakeRepo: newFakeRepo()}, "no-es-uuid", 0, validSettings(), "Zona Norte"
			},
			wantErr: domain.ErrZoneNotFound,
		},
		{
			name: "unknown zone is not found",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				return &failingRepo{fakeRepo: newFakeRepo()}, uuid.NewString(), 0, validSettings(), "Zona Norte"
			},
			wantErr: domain.ErrZoneNotFound,
		},
		{
			name: "find error is returned",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				return &failingRepo{fakeRepo: newFakeRepo(), findErr: errBoom}, uuid.NewString(), 0, validSettings(), "Zona Norte"
			},
			wantErr: errBoom,
		},
		{
			name: "stale version conflicts",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				repo, z := seeded(t)
				return &failingRepo{fakeRepo: repo}, z.ID, 7, validSettings(), "Zona Norte"
			},
			wantErr: domain.ErrVersionConflict,
		},
		{
			name: "inactive zone must be reactivated first",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				repo, z := seeded(t)
				z.Status = domain.StatusInactive
				repo.zones[z.ID] = z
				return &failingRepo{fakeRepo: repo}, z.ID, 0, validSettings(), "Zona Norte"
			},
			wantErr: domain.ErrZoneInactive,
		},
		{
			name: "invalid name is rejected before update",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				repo, z := seeded(t)
				return &failingRepo{fakeRepo: repo}, z.ID, 0, validSettings(), "@@"
			},
			wantErr: domain.ErrInvalidName,
		},
		{
			name: "invalid settings are rejected before update",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				repo, z := seeded(t)
				s := validSettings()
				s.MaxOrdersPerRoute = 0
				return &failingRepo{fakeRepo: repo}, z.ID, 0, s, "Zona Norte"
			},
			wantErr: domain.ErrInvalidSettings,
		},
		{
			name: "repository update error is wrapped",
			setup: func(t *testing.T) (*failingRepo, string, int, domain.Settings, string) {
				repo, z := seeded(t)
				return &failingRepo{fakeRepo: repo, updateErr: errBoom}, z.ID, 0, validSettings(), "Zona Norte"
			},
			wantErr: errBoom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, id, version, settings, name := tt.setup(t)
			svc := application.NewZoneService(repo)

			_, err := svc.UpdateZone(context.Background(), id, name, ring(t), nil, settings, version)

			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestSetZoneStatus_ChangesStatusWithReason(t *testing.T) {
	repo := newFakeRepo()
	existing := seedZone(t, repo, "NORTE")
	svc := application.NewZoneService(repo)

	zone, err := svc.SetZoneStatus(context.Background(), existing.ID, domain.StatusInactive, "  Obras en la avenida  ")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusInactive, zone.Status)
	assert.Equal(t, "Obras en la avenida", zone.StatusReason)
}

func TestSetZoneStatus_RejectsInvalidInput(t *testing.T) {
	repo := newFakeRepo()
	existing := seedZone(t, repo, "NORTE")

	tests := []struct {
		name    string
		id      string
		status  string
		reason  string
		repo    *failingRepo
		wantErr error
	}{
		{"invalid identifier", "no-es-uuid", domain.StatusInactive, "Motivo valido", nil, domain.ErrZoneNotFound},
		{"unknown status", existing.ID, "PAUSED", "Motivo valido", nil, domain.ErrInvalidStatus},
		{"reason too short", existing.ID, domain.StatusInactive, "no", nil, domain.ErrReasonRequired},
		{"unknown zone", uuid.NewString(), domain.StatusInactive, "Motivo valido", nil, domain.ErrZoneNotFound},
		{"same status is unchanged", existing.ID, domain.StatusActive, "Motivo valido", nil, domain.ErrStatusUnchanged},
		{"find error is returned", existing.ID, domain.StatusInactive, "Motivo valido", &failingRepo{fakeRepo: repo, findErr: errBoom}, errBoom},
		{"set status error is returned", existing.ID, domain.StatusInactive, "Motivo valido", &failingRepo{fakeRepo: repo, setStatusErr: errBoom}, errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.repo
			if r == nil {
				r = &failingRepo{fakeRepo: repo}
			}
			svc := application.NewZoneService(r)

			_, err := svc.SetZoneStatus(context.Background(), tt.id, tt.status, tt.reason)

			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestDeleteZone_DeletesInactiveZone(t *testing.T) {
	repo := newFakeRepo()
	existing := seedZone(t, repo, "NORTE")
	existing.Status = domain.StatusInactive
	repo.zones[existing.ID] = existing
	svc := application.NewZoneService(repo)

	err := svc.DeleteZone(context.Background(), existing.ID, "Zona retirada del servicio")

	require.NoError(t, err)
	assert.Empty(t, repo.zones)
}

func TestDeleteZone_ErrorCases(t *testing.T) {
	repo := newFakeRepo()
	active := seedZone(t, repo, "NORTE")

	tests := []struct {
		name    string
		id      string
		reason  string
		repo    *failingRepo
		wantErr error
	}{
		{"invalid identifier", "no-es-uuid", "Motivo valido", nil, domain.ErrZoneNotFound},
		{"reason too short", active.ID, "x", nil, domain.ErrReasonRequired},
		{"active zone cannot be deleted", active.ID, "Motivo valido", nil, domain.ErrZoneNotInactive},
		{"repository delete error is returned", active.ID, "Motivo valido", &failingRepo{fakeRepo: repo, deleteErr: errBoom}, errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.repo
			if r == nil {
				r = &failingRepo{fakeRepo: repo}
			}
			svc := application.NewZoneService(r)

			err := svc.DeleteZone(context.Background(), tt.id, tt.reason)

			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestGetZone_ReturnsStoredZone(t *testing.T) {
	repo := newFakeRepo()
	existing := seedZone(t, repo, "NORTE")
	svc := application.NewZoneService(repo)

	zone, err := svc.GetZone(context.Background(), existing.ID)

	require.NoError(t, err)
	assert.Equal(t, "NORTE", zone.Code)
}

func TestGetZone_ErrorCases(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())
	_, err := svc.GetZone(context.Background(), uuid.NewString())
	assert.ErrorIs(t, err, domain.ErrZoneNotFound)

	svc = application.NewZoneService(&failingRepo{fakeRepo: newFakeRepo(), findErr: errBoom})
	_, err = svc.GetZone(context.Background(), uuid.NewString())
	assert.ErrorIs(t, err, errBoom)
}

func TestListZones_ReturnsAllZones(t *testing.T) {
	repo := newFakeRepo()
	seedZone(t, repo, "NORTE")
	seedZone(t, repo, "SUR")
	svc := application.NewZoneService(repo)

	zones, err := svc.ListZones(context.Background())

	require.NoError(t, err)
	assert.Len(t, zones, 2)
}

func TestListZones_RepositoryErrorIsWrapped(t *testing.T) {
	svc := application.NewZoneService(&failingRepo{fakeRepo: newFakeRepo(), listErr: errBoom})

	_, err := svc.ListZones(context.Background())

	assert.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "list zones")
}

func TestClassifyPoint_ReturnsContainingZone(t *testing.T) {
	repo := newFakeRepo()
	seedZone(t, repo, "NORTE")
	svc := application.NewZoneService(repo)

	zone, err := svc.ClassifyPoint(context.Background(), -0.15, -78.45)

	require.NoError(t, err)
	assert.Equal(t, "NORTE", zone.Code)
}

func TestClassifyPoint_ErrorCases(t *testing.T) {
	svc := application.NewZoneService(newFakeRepo())
	_, err := svc.ClassifyPoint(context.Background(), -0.15, -78.45)
	assert.ErrorIs(t, err, domain.ErrZoneNotFound, "sin zonas no hay zona que contenga el punto")

	svc = application.NewZoneService(&failingRepo{fakeRepo: newFakeRepo(), containingErr: errBoom})
	_, err = svc.ClassifyPoint(context.Background(), -0.15, -78.45)
	assert.ErrorIs(t, err, errBoom)
}
