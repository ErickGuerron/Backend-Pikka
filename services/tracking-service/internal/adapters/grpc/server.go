// Package grpc exposes the position use case as the internal TrackingService gRPC API.
package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/application/position"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

// Tracker is the use case the server needs. *position.Tracker satisfies it.
type Tracker interface {
	RecordPosition(ctx context.Context, driverID string, latitude, longitude float64, recordedAt time.Time) (bool, error)
	EstimateArrival(ctx context.Context, driverID string, dest domain.Point) (domain.Estimate, error)
	FreshPosition(ctx context.Context, driverID string) (domain.Position, error)
}

// Server implements trackingv1.TrackingServiceServer on top of the use case.
type Server struct {
	trackingv1.UnimplementedTrackingServiceServer
	tracker Tracker
}

// NewServer wraps the use case.
func NewServer(tracker Tracker) *Server {
	return &Server{tracker: tracker}
}

// Register attaches the server to a gRPC server.
func Register(s *googlegrpc.Server, tracker Tracker) {
	trackingv1.RegisterTrackingServiceServer(s, NewServer(tracker))
}

// RecordPosition validates the request and stores the fix when it is newer than the stored one.
func (s *Server) RecordPosition(ctx context.Context, req *trackingv1.RecordPositionRequest) (*trackingv1.RecordPositionResponse, error) {
	recordedAt, err := time.Parse(time.RFC3339, req.GetRecordedAt())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "recorded_at must be RFC 3339")
	}
	accepted, err := s.tracker.RecordPosition(ctx, req.GetDriverId(), req.GetLatitude(), req.GetLongitude(), recordedAt)
	if err != nil {
		return nil, toStatus(err)
	}
	return &trackingv1.RecordPositionResponse{Accepted: accepted}, nil
}

// EstimateArrival estimates the road distance and time from the driver's latest fresh position.
func (s *Server) EstimateArrival(ctx context.Context, req *trackingv1.EstimateArrivalRequest) (*trackingv1.EstimateArrivalResponse, error) {
	dest := domain.Point{Latitude: req.GetDestinationLatitude(), Longitude: req.GetDestinationLongitude()}
	if !dest.Valid() {
		return nil, status.Error(codes.InvalidArgument, "destination coordinates out of range")
	}
	est, err := s.tracker.EstimateArrival(ctx, req.GetDriverId(), dest)
	if err != nil {
		return nil, toStatus(err)
	}
	return &trackingv1.EstimateArrivalResponse{DistanceMeters: est.DistanceMeters, DurationSeconds: est.DurationSeconds}, nil
}

// GetDriverPosition returns the driver's latest fresh fix. A missing or stale fix is NOT_FOUND: the
// caller shows no position instead of an old one.
func (s *Server) GetDriverPosition(ctx context.Context, req *trackingv1.GetDriverPositionRequest) (*trackingv1.GetDriverPositionResponse, error) {
	if strings.TrimSpace(req.GetDriverId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}
	fix, err := s.tracker.FreshPosition(ctx, req.GetDriverId())
	if errors.Is(err, position.ErrNoPosition) || errors.Is(err, position.ErrStalePosition) {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if err != nil {
		return nil, toStatus(err)
	}
	return &trackingv1.GetDriverPositionResponse{
		Latitude:   fix.Latitude,
		Longitude:  fix.Longitude,
		RecordedAt: fix.RecordedAt.UTC().Format(time.RFC3339),
	}, nil
}

// toStatus maps domain and use-case errors to gRPC codes. Unknown errors are Internal, never a
// fabricated "no route".
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidPosition), errors.Is(err, domain.ErrFutureTimestamp):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, position.ErrNoPosition), errors.Is(err, domain.ErrNoRoute):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, position.ErrStalePosition):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
