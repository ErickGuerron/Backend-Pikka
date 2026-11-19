package driver_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
)

// Plan: "confirmación simultánea" de la misma parada.
func TestPlan_RegisterDelivery_ConcurrentSameStop_ExactlyOneSucceeds(t *testing.T) {
	const callers = 16
	repo := memory.NewDriverRepository()
	recorder := memory.NewDeliveryRecorder()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, recorder, 50)

	start := make(chan struct{})
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, deliveryInput("stop-1", nearStop))
		}(i)
	}
	close(start)
	wg.Wait()

	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case assert.ErrorIs(t, err, ports.ErrDeliveryAlreadyRecorded):
			dup++
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, callers-1, dup)
	assert.Equal(t, 1, recorder.Count())
}

// Plan: registrar entrega de una parada que no está en ruta. Hoy el caso de uso solo
// puede rechazar por estado del repartidor (no ON_ROUTE) o por ruta distinta.
func TestPlan_RegisterDelivery_NotInRoute_Rejected(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(t *testing.T, repo *memory.DriverRepository)
		route string
		want  error
	}{
		{
			name: "repartidor disponible (sin ruta)",
			setup: func(t *testing.T, repo *memory.DriverRepository) {
				d, err := domain.NewDriver("d1", domaintest.Input("u"), t0)
				require.NoError(t, err)
				require.NoError(t, repo.Create(ctx, d))
			},
			route: "route-1",
			want:  domain.ErrInvalidTransition,
		},
		{
			name: "ruta reservada pero no iniciada",
			setup: func(t *testing.T, repo *memory.DriverRepository) {
				d, err := domain.NewDriver("d1", domaintest.Input("u"), t0)
				require.NoError(t, err)
				require.NoError(t, repo.Create(ctx, d))
				stored, err := repo.Get(ctx, "d1")
				require.NoError(t, err)
				require.NoError(t, stored.Reserve("route-1", t0))
				require.NoError(t, repo.Save(ctx, stored, stored.Version-1))
			},
			route: "route-1",
			want:  domain.ErrInvalidTransition,
		},
		{
			name:  "ruta cancelada / distinta a la activa",
			setup: func(t *testing.T, repo *memory.DriverRepository) { onRoute(t, repo, "d1", "route-1") },
			route: "route-cancelled",
			want:  domain.ErrRouteMismatch,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewDriverRepository()
			recorder := memory.NewDeliveryRecorder()
			tc.setup(t, repo)
			uc := driver.NewDeliveries(repo, recorder, 50)

			_, err := uc.RegisterDelivery(ctx, "d1", tc.route, stop, deliveryInput("stop-1", nearStop))

			assert.ErrorIs(t, err, tc.want)
			assert.Zero(t, recorder.Count())
		})
	}
}
