package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var errQueueDown = errors.New("zone queue unavailable")

// failingQueueRepo fails every position read, as an unavailable database would.
type failingQueueRepo struct {
	ports.ZoneQueueRepository
}

func (failingQueueRepo) QueuePosition(context.Context, domain.Order) (int, error) {
	return 0, errQueueDown
}

// failingListRepo fails the customer's order listing.
type failingListRepo struct {
	ports.OrderRepository
}

func (failingListRepo) ListByCustomer(context.Context, uuid.UUID) ([]domain.Order, error) {
	return nil, errQueueDown
}

// After routing, each zone and delivery date is filled once, not once per order.
func TestAssignFillsEachZoneDayOnce(t *testing.T) {
	date := mustDeliveryDate(t, "2030-04-12")
	first := domain.Order{ID: uuid.NewString(), ZoneID: "zone-1", DeliveryDate: date}
	second := domain.Order{ID: uuid.NewString(), ZoneID: "zone-1", DeliveryDate: date}
	other := domain.Order{ID: uuid.NewString(), ZoneID: "zone-2", DeliveryDate: date}
	repo := &routeRepo{result: []domain.Order{first, second, other}}
	zoneRepo := &fakeQueueRepo{}
	uc := NewRouteAssignment(repo).WithZoneQueue(NewZoneQueue(zoneRepo, &fakeCapacity{max: 2}))

	_, err := uc.Assign(context.Background(), AssignToRouteCommand{
		Role: RoleOperator, RouteID: "zone-1#1", OrderIDs: []string{first.ID, second.ID, other.ID},
	})
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	want := []fillCall{{"zone-1", "2030-04-12", 2}, {"zone-2", "2030-04-12", 2}}
	if len(zoneRepo.fillCalls) != len(want) {
		t.Fatalf("fill calls = %+v, want %+v", zoneRepo.fillCalls, want)
	}
	for i := range want {
		if zoneRepo.fillCalls[i] != want[i] {
			t.Fatalf("fill call %d = %+v, want %+v", i, zoneRepo.fillCalls[i], want[i])
		}
	}
}

// A repository failure while assigning must not fill any zone: nothing was routed.
func TestAssignRepositoryErrorFillsNothing(t *testing.T) {
	repo := &routeRepo{err: errQueueDown}
	zoneRepo := &fakeQueueRepo{}
	uc := NewRouteAssignment(repo).WithZoneQueue(NewZoneQueue(zoneRepo, &fakeCapacity{max: 2}))

	_, err := uc.Assign(context.Background(), AssignToRouteCommand{
		Role: RoleOperator, RouteID: "zone-1#1", OrderIDs: []string{uuid.NewString()},
	})
	if !errors.Is(err, errQueueDown) {
		t.Fatalf("Assign err = %v, want errQueueDown", err)
	}
	if len(zoneRepo.fillCalls) != 0 {
		t.Fatalf("fill calls = %+v, want none", zoneRepo.fillCalls)
	}
}

// The assignment is already committed when the fill runs, so a failed fill is logged and
// the assignment still succeeds; the next freeing event retries the fill.
func TestAssignSucceedsWhenZoneFillFails(t *testing.T) {
	order := domain.Order{ID: uuid.NewString(), ZoneID: "zone-1", DeliveryDate: mustDeliveryDate(t, "2030-04-12")}
	repo := &routeRepo{result: []domain.Order{order}}
	capacity := &fakeCapacity{err: errQueueDown}
	uc := NewRouteAssignment(repo).WithZoneQueue(NewZoneQueue(&fakeQueueRepo{}, capacity))

	got, err := uc.Assign(context.Background(), AssignToRouteCommand{
		Role: RoleOperator, RouteID: "zone-1#1", OrderIDs: []string{order.ID},
	})
	if err != nil {
		t.Fatalf("Assign: %v, want success despite the failed fill", err)
	}
	if len(got) != 1 || got[0].ID != order.ID {
		t.Fatalf("assigned = %+v, want the routed order", got)
	}
}

// A customer reading a waiting order gets its place in line, and a failed lookup is reported.
func TestGetMyOrderReportsQueueLookupFailure(t *testing.T) {
	customer := domain.Customer{ID: uuid.New()}
	id := uuid.NewString()
	orders := &getOrderRepo{orders: map[string]domain.Order{id: {
		ID: id, CustomerID: customer.ID.String(), Status: domain.StatusPending, ZoneID: "zone-1",
		DeliveryDate: mustDeliveryDate(t, "2030-04-12"),
	}}}
	uc := NewGetMyOrder(&queueCustomers{customer: customer}, orders).
		WithZoneQueue(NewZoneQueue(failingQueueRepo{}, &fakeCapacity{max: 2}))

	_, err := uc.Run(context.Background(), GetCommand{UserID: uuid.New(), Role: RoleCustomer, OrderID: id})
	if !errors.Is(err, errQueueDown) {
		t.Fatalf("Run err = %v, want errQueueDown", err)
	}
}

// Listing fails when the repository fails, and also when the queue lookup fails.
func TestListMyOrdersReportsFailures(t *testing.T) {
	customers := &queueCustomers{customer: domain.Customer{ID: uuid.New()}}
	cmd := Command{UserID: uuid.New(), Role: RoleCustomer}

	repoErr := NewListMyOrders(customers, failingListRepo{}).WithZoneQueue(NewZoneQueue(&fakeQueueRepo{}, &fakeCapacity{max: 2}))
	if _, err := repoErr.Run(context.Background(), cmd); !errors.Is(err, errQueueDown) {
		t.Fatalf("Run with failing repository err = %v, want errQueueDown", err)
	}

	waiting := domain.Order{
		ID: uuid.NewString(), Status: domain.StatusPending, ZoneID: "zone-1",
		DeliveryDate: mustDeliveryDate(t, "2030-04-12"),
	}
	queueErr := NewListMyOrders(customers, &queueListRepo{orders: []domain.Order{waiting}}).
		WithZoneQueue(NewZoneQueue(failingQueueRepo{}, &fakeCapacity{max: 2}))
	if _, err := queueErr.Run(context.Background(), cmd); !errors.Is(err, errQueueDown) {
		t.Fatalf("Run with failing queue err = %v, want errQueueDown", err)
	}
}

// A new order that waits for zone capacity is filled once and reports its place in line.
func TestCreateOrderAttachesQueueWhenWired(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})
	zoneRepo := &fakeQueueRepo{position: 3}
	uc := f.uc.WithZoneQueue(NewZoneQueue(zoneRepo, &fakeCapacity{max: 2}))

	got, err := uc.Run(context.Background(), knownPoint())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(zoneRepo.fillCalls) != 1 {
		t.Fatalf("fill calls = %d, want 1", len(zoneRepo.fillCalls))
	}
	if got.QueuePosition != 3 {
		t.Fatalf("queue position = %d, want 3", got.QueuePosition)
	}
}
