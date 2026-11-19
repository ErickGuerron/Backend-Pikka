package clients

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// DriverLocations implements ports.DriverLocations on top of Tracking Service.
type DriverLocations struct {
	client trackingv1.TrackingServiceClient
}

var _ ports.DriverLocations = (*DriverLocations)(nil)

// NewDriverLocations wraps the generated Tracking Service client.
func NewDriverLocations(client trackingv1.TrackingServiceClient) *DriverLocations {
	return &DriverLocations{client: client}
}

// LastPosition asks Tracking Service for the driver's fresh position. NotFound means the driver has no
// fix recent enough, which is a normal state and not an error; any other failure is returned, so an
// unavailable tracking service never looks like "no position".
func (d *DriverLocations) LastPosition(ctx context.Context, driverID string) (domain.Point, bool, error) {
	res, err := d.client.GetDriverPosition(ctx, &trackingv1.GetDriverPositionRequest{DriverId: driverID})
	if status.Code(err) == codes.NotFound {
		return domain.Point{}, false, nil
	}
	if err != nil {
		return domain.Point{}, false, fmt.Errorf("get driver position: %w", err)
	}
	return domain.Point{Latitude: res.GetLatitude(), Longitude: res.GetLongitude()}, true, nil
}
