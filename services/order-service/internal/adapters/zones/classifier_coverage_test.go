package zones

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// failingZoneClient falla GetZone con el error configurado.
type failingZoneClient struct {
	zonesv1.ZoneServiceClient
	err error
}

func (f failingZoneClient) GetZone(context.Context, *zonesv1.GetZoneRequest, ...grpc.CallOption) (*zonesv1.GetZoneResponse, error) {
	return nil, f.err
}

func TestNewClassifierKeepsTimeoutAndClient(t *testing.T) {
	c := NewClassifier(nil, 3*time.Second)

	if c == nil || c.client == nil {
		t.Fatal("NewClassifier must build a client")
	}
	if c.timeout != 3*time.Second {
		t.Fatalf("timeout = %s, want 3s", c.timeout)
	}
}

func TestClassifyPointOtherErrorsArePassedThrough(t *testing.T) {
	client := &fakeZoneClient{err: status.Error(codes.Internal, "boom")}

	_, err := classifierWith(client).ClassifyPoint(context.Background(), 0, 0)

	if status.Code(err) != codes.Internal {
		t.Fatalf("err = %v, want the original gRPC error", err)
	}
	if errors.Is(err, domain.ErrZoneNotCovered) || errors.Is(err, domain.ErrZonesUnavailable) {
		t.Fatalf("an unknown failure must not be reported as a zone problem: %v", err)
	}
}

func TestMaxOrdersPerRouteReturnsTheTransportError(t *testing.T) {
	boom := status.Error(codes.Unavailable, "zone service down")
	c := &Classifier{client: failingZoneClient{err: boom}, timeout: time.Second}

	got, err := c.MaxOrdersPerRoute(context.Background(), "zone-1")

	if got != 0 {
		t.Fatalf("capacity = %d, want 0 when the zone cannot be read", got)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the transport error", err)
	}
}
