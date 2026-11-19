package zones

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
)

// GetZone serves the zone's max_orders_per_route setting, the capacity of one driver's route in that zone.
func (f *fakeZoneClient) GetZone(_ context.Context, req *zonesv1.GetZoneRequest, _ ...grpc.CallOption) (*zonesv1.GetZoneResponse, error) {
	return &zonesv1.GetZoneResponse{Zone: &zonesv1.Zone{
		Id:       req.GetId(),
		Settings: &zonesv1.ZoneSettings{MaxOrdersPerRoute: f.maxPerRoute},
	}}, nil
}

func TestMaxOrdersPerRouteReadsTheZoneSetting(t *testing.T) {
	client := &fakeZoneClient{maxPerRoute: 7}

	got, err := classifierWith(client).MaxOrdersPerRoute(context.Background(), "zone-1")

	require.NoError(t, err)
	assert.Equal(t, 7, got)
}
