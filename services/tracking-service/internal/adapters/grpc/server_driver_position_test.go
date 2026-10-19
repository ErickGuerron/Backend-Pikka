package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

func TestGetDriverPosition_FreshFixIsReturned(t *testing.T) {
	recorded := clock.Add(-30 * time.Second)
	store := &memStore{m: map[string]domain.Position{
		"driver-1": {DriverID: "driver-1", Latitude: -0.18, Longitude: -78.48, RecordedAt: recorded},
	}}
	client := newClient(t, store, &fakeRoutes{})

	res, err := client.GetDriverPosition(context.Background(), &trackingv1.GetDriverPositionRequest{DriverId: "driver-1"})

	require.NoError(t, err)
	assert.InDelta(t, -0.18, res.GetLatitude(), 1e-9)
	assert.InDelta(t, -78.48, res.GetLongitude(), 1e-9)
	assert.Equal(t, recorded.Format(time.RFC3339), res.GetRecordedAt())
}

func TestGetDriverPosition_StaleOrUnknownIsNotFound(t *testing.T) {
	store := &memStore{m: map[string]domain.Position{
		"stale": {DriverID: "stale", Latitude: -0.18, Longitude: -78.48, RecordedAt: clock.Add(-3 * time.Minute)},
	}}
	client := newClient(t, store, &fakeRoutes{})

	for _, id := range []string{"stale", "nobody"} {
		_, err := client.GetDriverPosition(context.Background(), &trackingv1.GetDriverPositionRequest{DriverId: id})
		assert.Equal(t, codes.NotFound, status.Code(err), "driver %s", id)
	}
}

func TestGetDriverPosition_BlankDriverIsInvalidArgument(t *testing.T) {
	client := newClient(t, &memStore{m: map[string]domain.Position{}}, &fakeRoutes{})

	_, err := client.GetDriverPosition(context.Background(), &trackingv1.GetDriverPositionRequest{DriverId: "  "})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
