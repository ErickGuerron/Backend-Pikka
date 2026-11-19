package driver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
)

func TestFindByUserIDDevuelveElRepartidorDelUsuario(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	svc := driver.NewService(repo, nil)

	got, err := svc.FindByUserID(context.Background(), "user-d1")
	require.NoError(t, err)
	assert.Equal(t, "d1", got.ID)
	assert.Equal(t, "user-d1", got.UserID)
}

func TestFindByUserIDSinRepartidorEsNoEncontrado(t *testing.T) {
	svc := driver.NewService(memory.NewDriverRepository(), nil)

	_, err := svc.FindByUserID(context.Background(), "user-x")
	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}
