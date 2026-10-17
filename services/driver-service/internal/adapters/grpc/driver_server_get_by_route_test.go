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

func TestGetDriverByRouteIDDevuelveSoloIdentificadores(t *testing.T) {
	client, repo := setup(t)
	seedOnRoute(t, repo, "d1", "route-1")

	res, err := client.GetDriverByRouteID(context.Background(), &driversv1.GetDriverByRouteIDRequest{RouteId: "route-1"})
	require.NoError(t, err)
	assert.Equal(t, "d1", res.GetDriverId())
	assert.Equal(t, "user-d1", res.GetUserId())
}

func TestGetDriverByRouteIDSinRepartidorEsNotFound(t *testing.T) {
	client, repo := setup(t)
	seedOnRoute(t, repo, "d1", "route-1")

	_, err := client.GetDriverByRouteID(context.Background(), &driversv1.GetDriverByRouteIDRequest{RouteId: "route-2"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetDriverByRouteIDRouteIDEnBlancoEsInvalidArgument(t *testing.T) {
	client, _ := setup(t)

	_, err := client.GetDriverByRouteID(context.Background(), &driversv1.GetDriverByRouteIDRequest{RouteId: "   "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
