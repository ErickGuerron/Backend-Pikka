package grpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
)

func TestReleaseDriverCodigosYRespuesta(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	ctx := context.Background()
	_, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)

	tests := []struct {
		name     string
		driverID string
		routeID  string
		code     codes.Code
	}{
		{"driver_id blank", " ", "route-1", codes.InvalidArgument},
		{"route_id blank", "d1", "", codes.InvalidArgument},
		{"unknown driver", "nadie", "route-1", codes.NotFound},
		{"reserved for another route", "d1", "route-2", codes.FailedPrecondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: tt.driverID, RouteId: tt.routeID})
			assert.Equal(t, tt.code, status.Code(err))
		})
	}

	resp, err := client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, resp.GetDriver().GetStatus())

	again, err := client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err, "a repeated release is idempotent")
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, again.GetDriver().GetStatus())
}
