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

func TestStartRouteGRPCMovesTheDriverOnRoute(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	_, err := client.ReserveDriver(context.Background(), &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)

	res, err := client.StartRoute(context.Background(), &driversv1.StartRouteRequest{DriverId: "d1", RouteId: "route-1"})

	require.NoError(t, err)
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_ON_ROUTE, res.GetDriver().GetStatus())
}

func TestStartRouteGRPCRefusesAnotherRouteAsFailedPrecondition(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	_, err := client.ReserveDriver(context.Background(), &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)

	_, err = client.StartRoute(context.Background(), &driversv1.StartRouteRequest{DriverId: "d1", RouteId: "route-2"})

	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestStartRouteGRPCBlankIdsAreInvalidArgument(t *testing.T) {
	client, _ := setup(t)

	_, err := client.StartRoute(context.Background(), &driversv1.StartRouteRequest{DriverId: " ", RouteId: "route-1"})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
