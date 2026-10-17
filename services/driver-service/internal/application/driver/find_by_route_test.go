package driver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestFindByRouteIDDevuelveElRepartidorDeLaRuta(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	svc := driver.NewService(repo, nil)

	got, err := svc.FindByRouteID(context.Background(), "route-1")
	require.NoError(t, err)
	assert.Equal(t, "d1", got.ID)
	assert.Equal(t, "user-d1", got.UserID)
}

func TestFindByRouteIDSinRepartidorEsNoEncontrado(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	svc := driver.NewService(repo, nil)

	_, err := svc.FindByRouteID(context.Background(), "route-404")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestFindByRouteIDNoDevuelveAlRepartidorDeOtraRuta(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	svc := driver.NewService(repo, nil)

	_, err := svc.FindByRouteID(context.Background(), "route-2")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}

func TestFindByRouteIDRouteIDEnBlancoEsInvalido(t *testing.T) {
	svc := driver.NewService(memory.NewDriverRepository(), nil)

	for _, routeID := range []string{"", "   "} {
		_, err := svc.FindByRouteID(context.Background(), routeID)
		assert.ErrorIs(t, err, domain.ErrInvalidRouteID, "route_id %q", routeID)
	}
}
