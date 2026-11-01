package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/application/position"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

// failingPositions es un Tracker que responde con el error configurado en cada operación.
type failingPositions struct{ err error }

func (f failingPositions) RecordPosition(context.Context, string, float64, float64, time.Time) (bool, error) {
	return false, f.err
}

func (f failingPositions) EstimateArrival(context.Context, string, domain.Point) (domain.Estimate, error) {
	return domain.Estimate{}, f.err
}

func (f failingPositions) FreshPosition(context.Context, string) (domain.Position, error) {
	return domain.Position{}, f.err
}

func TestRegisterAttachesTheTrackingService(t *testing.T) {
	srv := googlegrpc.NewServer()
	defer srv.Stop()

	Register(srv, failingPositions{err: errors.New("unused")})

	info := srv.GetServiceInfo()
	if _, ok := info["tracking.v1.TrackingService"]; !ok {
		t.Fatalf("services = %v, want tracking.v1.TrackingService", info)
	}
}

func TestRecordPositionMapsUseCaseErrors(t *testing.T) {
	req := &trackingv1.RecordPositionRequest{DriverId: "driver-1", Latitude: 0, Longitude: 0, RecordedAt: clock.Format(time.RFC3339)}
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"invalid position", domain.ErrInvalidPosition, codes.InvalidArgument},
		{"future timestamp", domain.ErrFutureTimestamp, codes.InvalidArgument},
		{"unexpected store failure", errors.New("redis down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServer(failingPositions{err: tt.err}).RecordPosition(context.Background(), req)
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), tt.want, err)
			}
		})
	}
}

func TestEstimateArrivalMapsUseCaseErrors(t *testing.T) {
	req := &trackingv1.EstimateArrivalRequest{DriverId: "driver-1", DestinationLatitude: -0.2, DestinationLongitude: -78.5}
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"no fix", position.ErrNoPosition, codes.NotFound},
		{"no road route", domain.ErrNoRoute, codes.NotFound},
		{"stale fix", position.ErrStalePosition, codes.FailedPrecondition},
		{"unexpected failure", errors.New("osrm down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServer(failingPositions{err: tt.err}).EstimateArrival(context.Background(), req)
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), tt.want, err)
			}
		})
	}
}

func TestEstimateArrivalRejectsOutOfRangeDestinationBeforeUseCase(t *testing.T) {
	_, err := NewServer(failingPositions{err: errors.New("must not be called")}).EstimateArrival(context.Background(),
		&trackingv1.EstimateArrivalRequest{DriverId: "driver-1", DestinationLatitude: 95, DestinationLongitude: 0})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestGetDriverPositionMapsMissingStaleAndFailingFixes(t *testing.T) {
	req := &trackingv1.GetDriverPositionRequest{DriverId: "driver-1"}
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"no fix", position.ErrNoPosition, codes.NotFound},
		{"stale fix is not shown", position.ErrStalePosition, codes.NotFound},
		{"store failure", errors.New("redis down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServer(failingPositions{err: tt.err}).GetDriverPosition(context.Background(), req)
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), tt.want, err)
			}
		})
	}
}

func TestGetDriverPositionRejectsBlankDriverBeforeUseCase(t *testing.T) {
	_, err := NewServer(failingPositions{err: errors.New("must not be called")}).GetDriverPosition(context.Background(),
		&trackingv1.GetDriverPositionRequest{DriverId: "   "})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}
