package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestReleaseRouteReleasesTheReservedRoute(t *testing.T) {
	tests := []struct {
		name       string
		status     domain.Status
		reserved   string
		inactive   bool
		routeID    string
		wantErr    error
		wantStatus domain.Status
		wantRoute  string
		wantChange bool
	}{
		{"ASSIGNED to its route becomes AVAILABLE", domain.StatusAssigned, "route-1", false, "route-1", nil, domain.StatusAvailable, "", true},
		{"ON_ROUTE to its route becomes AVAILABLE", domain.StatusOnRoute, "route-1", false, "route-1", nil, domain.StatusAvailable, "", true},
		{"ASSIGNED and inactive becomes OFFLINE", domain.StatusAssigned, "route-1", true, "route-1", nil, domain.StatusOffline, "", true},
		{"ON_ROUTE and inactive becomes OFFLINE", domain.StatusOnRoute, "route-1", true, "route-1", nil, domain.StatusOffline, "", true},
		{"ASSIGNED to another route is refused", domain.StatusAssigned, "route-1", false, "route-2", domain.ErrRouteMismatch, domain.StatusAssigned, "route-1", false},
		{"ON_ROUTE to another route is refused", domain.StatusOnRoute, "route-1", false, "route-2", domain.ErrRouteMismatch, domain.StatusOnRoute, "route-1", false},
		{"blank route is refused", domain.StatusAssigned, "route-1", false, "   ", domain.ErrInvalidRouteID, domain.StatusAssigned, "route-1", false},
		{"AVAILABLE without route is a no-op", domain.StatusAvailable, "", false, "route-1", nil, domain.StatusAvailable, "", false},
		{"OFFLINE without route is a no-op", domain.StatusOffline, "", false, "route-1", nil, domain.StatusOffline, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDriver(t, tt.status)
			d.RouteID = tt.reserved
			if tt.inactive {
				d.Deactivate(t0)
			}
			before := d.Version

			err := d.ReleaseRoute(tt.routeID, t1)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStatus, d.Status)
			assert.Equal(t, tt.wantRoute, d.RouteID)
			if tt.wantChange {
				assert.Equal(t, before+1, d.Version, "a release is a state change: the version advances")
			} else {
				assert.Equal(t, before, d.Version, "a refused or idempotent release does not touch the driver")
			}
		})
	}
}

func TestReleaseRouteIsIdempotentAfterRelease(t *testing.T) {
	d := newDriver(t, domain.StatusAssigned)
	d.RouteID = "route-1"
	require.NoError(t, d.ReleaseRoute("route-1", t1))
	released := d.Version

	require.NoError(t, d.ReleaseRoute("route-1", t1), "a second release of the same route succeeds")

	assert.Equal(t, domain.StatusAvailable, d.Status)
	assert.Equal(t, released, d.Version)
}
