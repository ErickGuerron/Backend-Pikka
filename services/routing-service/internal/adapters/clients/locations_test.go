package clients

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// fakeTracking answers the last position of the drivers it knows; any other driver is NotFound.
type fakeTracking struct {
	trackingv1.UnimplementedTrackingServiceServer
	failWith error
	gotID    string
}

func (f *fakeTracking) GetDriverPosition(_ context.Context, req *trackingv1.GetDriverPositionRequest) (*trackingv1.GetDriverPositionResponse, error) {
	f.gotID = req.GetDriverId()
	if f.failWith != nil {
		return nil, f.failWith
	}
	if req.GetDriverId() != "cerca" {
		return nil, status.Error(codes.NotFound, "no fresh position")
	}
	return &trackingv1.GetDriverPositionResponse{Latitude: -1.24, Longitude: -78.62, RecordedAt: "2026-10-24T10:00:00Z"}, nil
}

func TestDriverLocationsReturnsTheFreshPosition(t *testing.T) {
	tracking := &fakeTracking{}
	conn := dial(t, func(s *grpc.Server) { trackingv1.RegisterTrackingServiceServer(s, tracking) })
	locations := NewDriverLocations(trackingv1.NewTrackingServiceClient(conn))

	p, ok, err := locations.LastPosition(context.Background(), "cerca")

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, domain.Point{Latitude: -1.24, Longitude: -78.62}, p)
	assert.Equal(t, "cerca", tracking.gotID)
}

func TestDriverLocationsNotFoundMeansNoFreshPosition(t *testing.T) {
	conn := dial(t, func(s *grpc.Server) { trackingv1.RegisterTrackingServiceServer(s, &fakeTracking{}) })
	locations := NewDriverLocations(trackingv1.NewTrackingServiceClient(conn))

	_, ok, err := locations.LastPosition(context.Background(), "sin-gps")

	require.NoError(t, err, "NotFound is a driver without a recent fix, not a failure")
	assert.False(t, ok)
}

func TestDriverLocationsPassesOtherErrorsThrough(t *testing.T) {
	tracking := &fakeTracking{failWith: status.Error(codes.Unavailable, "redis down")}
	conn := dial(t, func(s *grpc.Server) { trackingv1.RegisterTrackingServiceServer(s, tracking) })
	locations := NewDriverLocations(trackingv1.NewTrackingServiceClient(conn))

	_, _, err := locations.LastPosition(context.Background(), "cerca")

	require.Error(t, err, "an unavailable tracking service must not look like 'no position'")
}

// ringZones serves a zone with a closed square ring: its center is (2, 2).
type ringZones struct {
	zonesv1.UnimplementedZoneServiceServer
}

func (ringZones) GetZone(_ context.Context, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	ring := []*zonesv1.Coordinate{
		{Latitude: 1, Longitude: 1}, {Latitude: 1, Longitude: 3}, {Latitude: 3, Longitude: 3},
		{Latitude: 3, Longitude: 1}, {Latitude: 1, Longitude: 1},
	}
	return &zonesv1.GetZoneResponse{Zone: &zonesv1.Zone{Id: req.GetId(), Ring: ring}}, nil
}

func TestZoneCapacitiesCenterIsTheRingCentroid(t *testing.T) {
	conn := dial(t, func(s *grpc.Server) { zonesv1.RegisterZoneServiceServer(s, ringZones{}) })
	zones := NewZoneCapacities(zonesv1.NewZoneServiceClient(conn))

	center, ok, err := zones.ZoneCenter(context.Background(), "A")

	require.NoError(t, err)
	require.True(t, ok)
	assert.InDelta(t, 2.0, center.Latitude, 1e-9)
	assert.InDelta(t, 2.0, center.Longitude, 1e-9)
	assert.False(t, math.IsNaN(center.Latitude))
}
