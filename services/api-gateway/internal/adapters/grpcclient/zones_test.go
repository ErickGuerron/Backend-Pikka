package grpcclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
)

// fakeHealth responde un estado fijo al health check de gRPC.
type fakeHealth struct {
	status healthpb.HealthCheckResponse_ServingStatus
	err    error
}

func (f fakeHealth) Check(context.Context, *healthpb.HealthCheckRequest, ...grpc.CallOption) (*healthpb.HealthCheckResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &healthpb.HealthCheckResponse{Status: f.status}, nil
}

func (f fakeHealth) List(context.Context, *healthpb.HealthListRequest, ...grpc.CallOption) (*healthpb.HealthListResponse, error) {
	return nil, errors.New("not implemented")
}

func (f fakeHealth) Watch(context.Context, *healthpb.HealthCheckRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[healthpb.HealthCheckResponse], error) {
	return nil, errors.New("not implemented")
}

func TestZonesReady_OnlyWhenServing(t *testing.T) {
	cases := []struct {
		name   string
		health fakeHealth
		want   bool
	}{
		{"sirviendo", fakeHealth{status: healthpb.HealthCheckResponse_SERVING}, true},
		{"no sirve", fakeHealth{status: healthpb.HealthCheckResponse_NOT_SERVING}, false},
		{"inalcanzable", fakeHealth{err: errors.New("unavailable")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			z := &Zones{health: tc.health, timeout: time.Second}
			assert.Equal(t, tc.want, z.Ready(context.Background()))
		})
	}
}

// coverageClient captura la metadata saliente de ListCoverageZones.
type coverageClient struct {
	zonesv1.ZoneServiceClient
	md metadata.MD
}

func (c *coverageClient) ListCoverageZones(ctx context.Context, _ *zonesv1.ListCoverageZonesRequest, _ ...grpc.CallOption) (*zonesv1.ListCoverageZonesResponse, error) {
	c.md, _ = metadata.FromOutgoingContext(ctx)
	return &zonesv1.ListCoverageZonesResponse{Zones: []*zonesv1.Zone{{Id: "z-1"}}}, nil
}

func TestZonesListCoverageZones_PropagatesIdentity(t *testing.T) {
	client := &coverageClient{}
	z := NewZonesWithClient(client, time.Second)

	res, err := z.ListCoverageZones(context.Background(), CallMeta{RequestID: "rid-1", UserID: "u-1", Role: "CUSTOMER"}, &zonesv1.ListCoverageZonesRequest{})

	require.NoError(t, err)
	assert.Len(t, res.GetZones(), 1)
	assert.Equal(t, []string{"CUSTOMER"}, client.md.Get(MetadataUserRole))
	assert.Equal(t, []string{"u-1"}, client.md.Get(MetadataUserID))
	assert.Equal(t, []string{"rid-1"}, client.md.Get(MetadataRequestID))
}
