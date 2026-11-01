package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func driver(id, userID, first, last string) *domain.Driver {
	return &domain.Driver{
		ID: id, UserID: userID, FirstName: first, LastName: last,
		Identifier: "ID-" + id, Plate: "PL-" + id,
		Active: true, Status: domain.StatusAvailable,
	}
}

func TestCreateStoresDriverAndRejectsDuplicates(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	require.NoError(t, repo.Create(ctx, driver("d1", "u1", "Ana", "Ruiz")))

	tests := map[string]*domain.Driver{
		"same id":         driver("d1", "u2", "Luis", "Mora"),
		"same user id":    driver("d2", "u1", "Luis", "Mora"),
		"same identifier": {ID: "d3", UserID: "u3", Identifier: "ID-d1", Plate: "PL-d3"},
		"same plate":      {ID: "d4", UserID: "u4", Identifier: "ID-d4", Plate: "PL-d1"},
	}
	for name, dup := range tests {
		t.Run(name, func(t *testing.T) {
			err := repo.Create(ctx, dup)
			assert.ErrorIs(t, err, ports.ErrDriverExists)
		})
	}
}

func TestCreateStoresACopy(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	d := driver("d1", "u1", "Ana", "Ruiz")
	require.NoError(t, repo.Create(ctx, d))

	d.FirstName = "Modificada"

	got, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, "Ana", got.FirstName, "el repositorio guarda una copia, no el puntero del llamador")
}

func TestGetReturnsCopyOrNotFound(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	require.NoError(t, repo.Create(ctx, driver("d1", "u1", "Ana", "Ruiz")))

	got, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, "u1", got.UserID)

	_, err = repo.Get(ctx, "missing")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestFindByUserIDAndRouteID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	reserved := driver("d1", "u1", "Ana", "Ruiz")
	reserved.RouteID = "route-9"
	require.NoError(t, repo.Create(ctx, reserved))

	byUser, err := repo.FindByUserID(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, "d1", byUser.ID)

	_, err = repo.FindByUserID(ctx, "nobody")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	byRoute, err := repo.FindByRouteID(ctx, "route-9")
	require.NoError(t, err)
	assert.Equal(t, "d1", byRoute.ID)

	_, err = repo.FindByRouteID(ctx, "route-404")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	_, err = repo.FindByRouteID(ctx, "")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound, "un repartidor sin ruta nunca coincide con una ruta vacía")
}

func TestListSortsByLastNameFirstNameAndID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	require.NoError(t, repo.Create(ctx, driver("d3", "u3", "Ana", "Zambrano")))
	require.NoError(t, repo.Create(ctx, driver("d2", "u2", "Luis", "Mora")))
	require.NoError(t, repo.Create(ctx, driver("d1", "u1", "Ana", "Mora")))
	require.NoError(t, repo.Create(ctx, driver("d4", "u4", "Ana", "Mora")))

	all, err := repo.List(ctx, 10)
	require.NoError(t, err)
	var ids []string
	for _, d := range all {
		ids = append(ids, d.ID)
	}
	assert.Equal(t, []string{"d1", "d4", "d2", "d3"}, ids, "apellido, luego nombre, luego ID")

	limited, err := repo.List(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}

func TestSaveRequiresExistingDriverAndMatchingVersion(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	d := driver("d1", "u1", "Ana", "Ruiz")
	d.Version = 3
	require.NoError(t, repo.Create(ctx, d))

	updated := driver("d1", "u1", "Ana", "Ruiz")
	updated.Version = 4
	updated.Status = domain.StatusOffline

	err := repo.Save(ctx, updated, 2)
	assert.ErrorIs(t, err, ports.ErrVersionConflict)

	err = repo.Save(ctx, driver("missing", "u9", "X", "Y"), 0)
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)

	require.NoError(t, repo.Save(ctx, updated, 3))
	got, err := repo.Get(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOffline, got.Status)
}

func TestFindAvailableReturnsOnlyActiveAvailableDriversUpToLimit(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDriverRepository()
	require.NoError(t, repo.Create(ctx, driver("d1", "u1", "A", "A")))
	busy := driver("d2", "u2", "B", "B")
	busy.Status = domain.StatusOffline
	require.NoError(t, repo.Create(ctx, busy))
	inactive := driver("d3", "u3", "C", "C")
	inactive.Active = false
	require.NoError(t, repo.Create(ctx, inactive))
	require.NoError(t, repo.Create(ctx, driver("d4", "u4", "D", "D")))
	require.NoError(t, repo.Create(ctx, driver("d5", "u5", "E", "E")))

	got, err := repo.FindAvailable(ctx, 10)
	require.NoError(t, err)
	var ids []string
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	assert.Equal(t, []string{"d1", "d4", "d5"}, ids)

	limited, err := repo.FindAvailable(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
	assert.Equal(t, "d1", limited[0].ID)
}

func TestDeliveryRecorderCountsAndRejectsDuplicateStops(t *testing.T) {
	ctx := context.Background()
	rec := memory.NewDeliveryRecorder()
	assert.Equal(t, 0, rec.Count())

	require.NoError(t, rec.Record(ctx, "route-1", domain.DeliveryRecord{StopID: "stop-1"}, domain.ArrivalCheck{}))
	assert.Equal(t, 1, rec.Count())

	err := rec.Record(ctx, "route-1", domain.DeliveryRecord{StopID: "stop-1"}, domain.ArrivalCheck{})
	assert.ErrorIs(t, err, ports.ErrDeliveryAlreadyRecorded)
	assert.Equal(t, 1, rec.Count(), "un duplicado no suma")

	require.NoError(t, rec.Record(ctx, "route-2", domain.DeliveryRecord{StopID: "stop-1"}, domain.ArrivalCheck{}),
		"la misma parada en otra ruta es otro registro")
	assert.Equal(t, 2, rec.Count())
}
