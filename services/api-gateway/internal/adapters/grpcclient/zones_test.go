package grpcclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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
