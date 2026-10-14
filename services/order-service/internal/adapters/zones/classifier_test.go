package zones

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// fakeZoneClient responde con un error o una zona, y registra la metadata recibida.
type fakeZoneClient struct {
	zonesv1.ZoneServiceClient
	err      error
	zoneID   string
	metadata metadata.MD
}

func (f *fakeZoneClient) ClassifyPoint(ctx context.Context, _ *zonesv1.ClassifyPointRequest, _ ...grpc.CallOption) (*zonesv1.ClassifyPointResponse, error) {
	f.metadata, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.ClassifyPointResponse{Zone: &zonesv1.Zone{Id: f.zoneID}}, nil
}

func classifierWith(client *fakeZoneClient) *Classifier {
	return &Classifier{client: client, timeout: time.Second}
}

func TestClassifyPointReturnsZoneID(t *testing.T) {
	client := &fakeZoneClient{zoneID: "zone-1"}
	id, err := classifierWith(client).ClassifyPoint(context.Background(), -0.18, -78.48)
	if err != nil {
		t.Fatalf("ClassifyPoint: %v", err)
	}
	if id != "zone-1" {
		t.Fatalf("id = %q, want zone-1", id)
	}
	if got := client.metadata.Get(metadataUserRole); len(got) != 1 || got[0] != serviceRole {
		t.Fatalf("outgoing role = %v, want %s", got, serviceRole)
	}
}

func TestClassifyPointNotFoundMeansOutsideEveryZone(t *testing.T) {
	client := &fakeZoneClient{err: status.Error(codes.NotFound, "no zone")}
	_, err := classifierWith(client).ClassifyPoint(context.Background(), 0, 0)
	if !errors.Is(err, domain.ErrZoneNotCovered) {
		t.Fatalf("err = %v, want ErrZoneNotCovered", err)
	}
}

func TestClassifyPointUnavailableIsNotSilentlyUncovered(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded} {
		client := &fakeZoneClient{err: status.Error(code, "down")}
		_, err := classifierWith(client).ClassifyPoint(context.Background(), 0, 0)
		if !errors.Is(err, domain.ErrZonesUnavailable) {
			t.Fatalf("code %s: err = %v, want ErrZonesUnavailable", code, err)
		}
	}
}
