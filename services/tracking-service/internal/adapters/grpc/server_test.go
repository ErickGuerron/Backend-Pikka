package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/application/position"
	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

var clock = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type memStore struct{ m map[string]domain.Position }

func (s *memStore) Save(_ context.Context, p domain.Position, _ time.Duration) error {
	s.m[p.DriverID] = p
	return nil
}

func (s *memStore) Latest(_ context.Context, id string) (domain.Position, bool, error) {
	p, ok := s.m[id]
	return p, ok, nil
}

type fakeRoutes struct {
	out domain.Estimate
	err error
}

func (f *fakeRoutes) Estimate(context.Context, domain.Point, domain.Point) (domain.Estimate, error) {
	return f.out, f.err
}

// newClient serves the real Server over bufconn, so status codes travel the wire like in production.
func newClient(t *testing.T, store *memStore, routes *fakeRoutes) trackingv1.TrackingServiceClient {
	t.Helper()
	tracker := position.NewTracker(store, routes, func() time.Time { return clock })
	lis := bufconn.Listen(1 << 20)
	srv := googlegrpc.NewServer()
	trackingv1.RegisterTrackingServiceServer(srv, NewServer(tracker))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := googlegrpc.NewClient("passthrough:///bufnet",
		googlegrpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return trackingv1.NewTrackingServiceClient(conn)
}

func codeOf(err error) codes.Code { return status.Code(err) }

func TestRecordPositionAcceptsFreshFix(t *testing.T) {
	c := newClient(t, &memStore{m: map[string]domain.Position{}}, &fakeRoutes{})

	resp, err := c.RecordPosition(context.Background(), &trackingv1.RecordPositionRequest{
		DriverId: "driver-1", Latitude: -0.18, Longitude: -78.48, RecordedAt: clock.Format(time.RFC3339),
	})
	if err != nil || !resp.GetAccepted() {
		t.Fatalf("accepted=%v err=%v", resp.GetAccepted(), err)
	}
}

func TestRecordPositionRejectsBadInputAsInvalidArgument(t *testing.T) {
	c := newClient(t, &memStore{m: map[string]domain.Position{}}, &fakeRoutes{})
	cases := map[string]*trackingv1.RecordPositionRequest{
		"blank driver":     {DriverId: " ", Latitude: 0, Longitude: 0, RecordedAt: clock.Format(time.RFC3339)},
		"latitude range":   {DriverId: "d", Latitude: 91, Longitude: 0, RecordedAt: clock.Format(time.RFC3339)},
		"malformed time":   {DriverId: "d", Latitude: 0, Longitude: 0, RecordedAt: "16/10/2026"},
		"future timestamp": {DriverId: "d", Latitude: 0, Longitude: 0, RecordedAt: clock.Add(time.Minute).Format(time.RFC3339)},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := c.RecordPosition(context.Background(), req); codeOf(err) != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument (err=%v)", codeOf(err), err)
			}
		})
	}
}

func TestEstimateArrivalReturnsDistanceAndDuration(t *testing.T) {
	store := &memStore{m: map[string]domain.Position{}}
	fresh, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock.Add(-5*time.Second), clock)
	store.m["driver-1"] = fresh
	c := newClient(t, store, &fakeRoutes{out: domain.Estimate{DistanceMeters: 6283, DurationSeconds: 653}})

	resp, err := c.EstimateArrival(context.Background(), &trackingv1.EstimateArrivalRequest{
		DriverId: "driver-1", DestinationLatitude: -0.22, DestinationLongitude: -78.50,
	})
	if err != nil {
		t.Fatalf("EstimateArrival: %v", err)
	}
	if resp.GetDistanceMeters() != 6283 || resp.GetDurationSeconds() != 653 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestEstimateArrivalWithoutPositionIsNotFound(t *testing.T) {
	c := newClient(t, &memStore{m: map[string]domain.Position{}}, &fakeRoutes{})

	_, err := c.EstimateArrival(context.Background(), &trackingv1.EstimateArrivalRequest{
		DriverId: "driver-1", DestinationLatitude: -0.22, DestinationLongitude: -78.50,
	})
	if codeOf(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound (err=%v)", codeOf(err), err)
	}
}

func TestEstimateArrivalWithStalePositionIsFailedPrecondition(t *testing.T) {
	store := &memStore{m: map[string]domain.Position{}}
	stale, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock.Add(-200*time.Second), clock)
	store.m["driver-1"] = stale
	c := newClient(t, store, &fakeRoutes{})

	_, err := c.EstimateArrival(context.Background(), &trackingv1.EstimateArrivalRequest{
		DriverId: "driver-1", DestinationLatitude: -0.22, DestinationLongitude: -78.50,
	})
	if codeOf(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (err=%v)", codeOf(err), err)
	}
}

func TestEstimateArrivalNoRouteIsNotFound(t *testing.T) {
	store := &memStore{m: map[string]domain.Position{}}
	fresh, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock, clock)
	store.m["driver-1"] = fresh
	c := newClient(t, store, &fakeRoutes{err: domain.ErrNoRoute})

	_, err := c.EstimateArrival(context.Background(), &trackingv1.EstimateArrivalRequest{
		DriverId: "driver-1", DestinationLatitude: 4.7, DestinationLongitude: -74.0,
	})
	if codeOf(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound (err=%v)", codeOf(err), err)
	}
}

func TestEstimateArrivalRejectsInvalidDestination(t *testing.T) {
	store := &memStore{m: map[string]domain.Position{}}
	fresh, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock, clock)
	store.m["driver-1"] = fresh
	c := newClient(t, store, &fakeRoutes{})

	_, err := c.EstimateArrival(context.Background(), &trackingv1.EstimateArrivalRequest{
		DriverId: "driver-1", DestinationLatitude: 200, DestinationLongitude: 0,
	})
	if codeOf(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument (err=%v)", codeOf(err), err)
	}
}

// An unexpected failure of the routing engine must not leak as a fake "no route".
func TestEstimateArrivalEngineFailureIsInternal(t *testing.T) {
	store := &memStore{m: map[string]domain.Position{}}
	fresh, _ := domain.NewPosition("driver-1", -0.18, -78.48, clock, clock)
	store.m["driver-1"] = fresh
	c := newClient(t, store, &fakeRoutes{err: errors.New("osrm call: connection refused")})

	_, err := c.EstimateArrival(context.Background(), &trackingv1.EstimateArrivalRequest{
		DriverId: "driver-1", DestinationLatitude: -0.22, DestinationLongitude: -78.50,
	})
	if codeOf(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal (err=%v)", codeOf(err), err)
	}
}
