package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestDeactivateSegunEstado(t *testing.T) {
	tests := []struct {
		name       string
		status     domain.Status
		wantStatus domain.Status
		wantRoute  string
	}{
		{name: "AVAILABLE pasa a OFFLINE", status: domain.StatusAvailable, wantStatus: domain.StatusOffline},
		{name: "OFFLINE se mantiene", status: domain.StatusOffline, wantStatus: domain.StatusOffline},
		{name: "ASSIGNED conserva la reserva", status: domain.StatusAssigned, wantStatus: domain.StatusAssigned, wantRoute: "route-1"},
		{name: "ON_ROUTE conserva la ruta", status: domain.StatusOnRoute, wantStatus: domain.StatusOnRoute, wantRoute: "route-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDriver(t, tt.status)
			before := d.Version

			d.Deactivate(t1)

			assert.False(t, d.Active)
			assert.Equal(t, tt.wantStatus, d.Status)
			assert.Equal(t, tt.wantRoute, d.RouteID)
			assert.Equal(t, before+1, d.Version)
			assert.Equal(t, t1, d.UpdatedAt)
		})
	}
}

func TestDeactivateEsIdempotente(t *testing.T) {
	d := newDriver(t, domain.StatusAvailable)
	d.Deactivate(t1)
	version := d.Version

	d.Deactivate(t1.Add(time.Minute))

	assert.Equal(t, version, d.Version)
	assert.Equal(t, t1, d.UpdatedAt)
	assert.Equal(t, domain.StatusOffline, d.Status)
}

func TestActivateDejaOfflineYSubeVersion(t *testing.T) {
	d := newDriver(t, domain.StatusAvailable)
	d.Deactivate(t1)
	version := d.Version

	d.Activate(t1.Add(time.Hour))

	assert.True(t, d.Active)
	assert.Equal(t, domain.StatusOffline, d.Status, "reactivar no pone al repartidor disponible")
	assert.Equal(t, version+1, d.Version)
	require.NoError(t, d.GoOnline(t1.Add(2*time.Hour)), "tras activar debe poder volver a conectarse")
	assert.Equal(t, domain.StatusAvailable, d.Status)
}

func TestActivateEsIdempotente(t *testing.T) {
	d := newDriver(t, domain.StatusAvailable)
	version := d.Version

	d.Activate(t1)

	assert.Equal(t, version, d.Version)
	assert.Equal(t, t0, d.UpdatedAt)
}

func TestDriverInactivoNoPuedeReservarNiConectarse(t *testing.T) {
	d := newDriver(t, domain.StatusOffline)
	d.Deactivate(t1)

	assert.ErrorIs(t, d.GoOnline(t1), domain.ErrDriverInactive)
	assert.ErrorIs(t, d.Reserve("route-9", t1), domain.ErrDriverInactive)
}

func TestDeactivateEnRutaTerminaOfflineAlCompletar(t *testing.T) {
	d := newDriver(t, domain.StatusOnRoute)
	d.Deactivate(t1)
	require.NoError(t, d.CompleteRoute("route-1", t1))

	assert.Equal(t, domain.StatusOffline, d.Status)
	assert.Empty(t, d.RouteID)
}
