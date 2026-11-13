package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// fakeQueueRepo records what the queue asks of persistence and returns canned answers.
type fakeQueueRepo struct {
	ports.ZoneQueueRepository
	fillCalls []fillCall
	assigned  int
	active    int
	position  int
	positions int
	days      []domain.ZoneDay
}

type fillCall struct {
	zoneID, deliveryDate string
	maxPerRoute          int
}

func (r *fakeQueueRepo) FillZoneQueue(_ context.Context, zoneID, deliveryDate string, limits domain.ZoneLimits) (int, error) {
	r.fillCalls = append(r.fillCalls, fillCall{zoneID, deliveryDate, limits.PerRoute})
	return r.assigned, nil
}

func (r *fakeQueueRepo) QueuePosition(_ context.Context, _ domain.Order) (int, error) {
	r.positions++
	return r.position, nil
}

type fakeCapacity struct {
	max  int
	err  error
	zone string
}

func (c *fakeCapacity) ZoneLimits(_ context.Context, zoneID string) (domain.ZoneLimits, error) {
	c.zone = zoneID
	return domain.ZoneLimits{Daily: c.max, PerRoute: c.max}, c.err
}

func TestZoneQueueFillPassesZoneDateAndCapacity(t *testing.T) {
	repo := &fakeQueueRepo{assigned: 2}
	queue := NewZoneQueue(repo, &fakeCapacity{max: 3})

	got, err := queue.Fill(context.Background(), "zone-1", "2030-04-12")
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if got != 2 {
		t.Fatalf("Fill assigned %d, want 2", got)
	}
	want := fillCall{"zone-1", "2030-04-12", 3}
	if len(repo.fillCalls) != 1 || repo.fillCalls[0] != want {
		t.Fatalf("fill calls = %+v, want one call %+v", repo.fillCalls, want)
	}
}

func TestZoneQueueFillSkipsWithoutZoneOrDate(t *testing.T) {
	repo := &fakeQueueRepo{}
	capacity := &fakeCapacity{max: 3}
	queue := NewZoneQueue(repo, capacity)

	for _, c := range []struct{ zone, date string }{{"", "2030-04-12"}, {"zone-1", ""}} {
		if _, err := queue.Fill(context.Background(), c.zone, c.date); err != nil {
			t.Fatalf("Fill(%q,%q): %v", c.zone, c.date, err)
		}
	}
	if len(repo.fillCalls) != 0 {
		t.Fatalf("repository called %d times for an order without zone or date", len(repo.fillCalls))
	}
}

// A zone whose routes hold no orders at all (capacity 0) must not touch any pending order.
func TestZoneQueueFillSkipsZeroCapacity(t *testing.T) {
	repo := &fakeQueueRepo{}
	queue := NewZoneQueue(repo, &fakeCapacity{max: 0})

	if _, err := queue.Fill(context.Background(), "zone-1", "2030-04-12"); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(repo.fillCalls) != 0 {
		t.Fatalf("repository called with zero capacity: %+v", repo.fillCalls)
	}
}

func TestZoneQueueFillReturnsCapacityError(t *testing.T) {
	boom := errors.New("zone service down")
	repo := &fakeQueueRepo{}
	queue := NewZoneQueue(repo, &fakeCapacity{err: boom})

	if _, err := queue.Fill(context.Background(), "zone-1", "2030-04-12"); !errors.Is(err, boom) {
		t.Fatalf("Fill error = %v, want %v", err, boom)
	}
	if len(repo.fillCalls) != 0 {
		t.Fatal("repository must not run when capacity is unknown")
	}
}

// Only a PENDING order without route and with a zone is waiting in line. Anything else has
// position 0 and must not reach the repository.
func TestZoneQueuePositionOnlyForWaitingOrders(t *testing.T) {
	repo := &fakeQueueRepo{position: 3, active: 3}
	queue := NewZoneQueue(repo, &fakeCapacity{max: 3})
	ctx := context.Background()

	waiting := domain.Order{ID: uuid.NewString(), Status: domain.StatusPending, ZoneID: "zone-1", DeliveryDate: mustDeliveryDate(t, "2030-04-12")}
	if got, err := queue.Position(ctx, waiting); err != nil || got != 3 {
		t.Fatalf("waiting position = %d, %v; want 3, nil", got, err)
	}

	notWaiting := []domain.Order{
		{ID: uuid.NewString(), Status: domain.StatusRouteAssigned, ZoneID: "zone-1", RouteID: "zone-1#1"},
		{ID: uuid.NewString(), Status: domain.StatusPending, ZoneID: "zone-1", RouteID: "zone-1#1"},
		{ID: uuid.NewString(), Status: domain.StatusPending, ZoneID: ""},
	}
	for _, o := range notWaiting {
		if got, err := queue.Position(ctx, o); err != nil || got != 0 {
			t.Fatalf("position for %s with route %q zone %q = %d, %v; want 0", o.Status, o.RouteID, o.ZoneID, got, err)
		}
	}
	if repo.positions != 1 {
		t.Fatalf("repository asked %d times, want 1 (only the waiting order)", repo.positions)
	}
}

func (r *fakeQueueRepo) WaitingZoneDays(context.Context) ([]domain.ZoneDay, error) {
	return r.days, nil
}

func (r *fakeQueueRepo) ZoneActiveCount(context.Context, string, string) (int, error) {
	return r.active, nil
}

func TestZoneQueueSweepFillsEachPendingScope(t *testing.T) {
	repo := &fakeQueueRepo{assigned: 1, days: []domain.ZoneDay{
		{ZoneID: "a", DeliveryDate: "2030-04-12"}, {ZoneID: "a", DeliveryDate: "2030-04-13"},
		{ZoneID: "b", DeliveryDate: "2030-04-12"},
	}}
	queue := NewZoneQueue(repo, &fakeCapacity{max: 4})
	count, err := queue.Sweep(context.Background())
	if err != nil || count != 3 || len(repo.fillCalls) != 3 {
		t.Fatalf("sweep returned %d, %v; calls: %+v", count, err, repo.fillCalls)
	}
	for i, call := range repo.fillCalls {
		if call.zoneID != repo.days[i].ZoneID || call.deliveryDate != repo.days[i].DeliveryDate {
			t.Fatalf("scope %d = %+v", i, call)
		}
	}
}
