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

func TestPlanningAdmitsOnlyOldestOrdersWithinZoneCapacity(t *testing.T) {
	date := mustDeliveryDate(t, "2030-04-12")
	first := domain.Order{ID: uuid.NewString(), ZoneID: "zone-1", DeliveryDate: date}
	second := domain.Order{ID: uuid.NewString(), ZoneID: "zone-1", DeliveryDate: date}
	third := domain.Order{ID: uuid.NewString(), ZoneID: "zone-1", DeliveryDate: date}
	other := domain.Order{ID: uuid.NewString(), ZoneID: "zone-2", DeliveryDate: date}
	repo := &routeRepo{result: []domain.Order{first, second, third, other}}
	queue := NewZoneQueue(&fakeQueueRepo{active: 1}, &fakeCapacity{max: 3})
	uc := NewRouteAssignment(repo).WithZoneQueue(queue, nil)
	got, err := uc.PendingByDate(context.Background(), RoleOperator, date.String())
	if err != nil || len(got) != 3 || got[0].ID != first.ID || got[1].ID != second.ID || got[2].ID != other.ID {
		t.Fatalf("eligible orders = %+v, error = %v", got, err)
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
	zoneRepo := &fakeQueueRepo{position: 3, active: 2}
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
