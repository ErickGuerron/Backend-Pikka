package order

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// queueCustomers resolves the single customer these tests act as.
type queueCustomers struct {
	ports.CustomerRepository
	customer domain.Customer
}

func (c *queueCustomers) GetByUserID(_ context.Context, _ uuid.UUID) (domain.Customer, error) {
	return c.customer, nil
}

// queueCancelRepo hands the current order to the cancel decision, as the repository does.
type queueCancelRepo struct {
	ports.OrderRepository
	current domain.Order
}

func (r *queueCancelRepo) CancelForCustomer(_ context.Context, _ string, _ uuid.UUID, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	return decide(r.current)
}

// queueListRepo returns the customer's orders for the read path.
type queueListRepo struct {
	ports.OrderRepository
	orders []domain.Order
}

func (r *queueListRepo) ListByCustomer(_ context.Context, _ uuid.UUID) ([]domain.Order, error) {
	return r.orders, nil
}

func mustDeliveryDate(t *testing.T, value string) domain.DeliveryDate {
	t.Helper()
	date, err := domain.ParseDeliveryDate(value)
	if err != nil {
		t.Fatalf("ParseDeliveryDate(%q): %v", value, err)
	}
	return date
}

// The stop closure frees one route slot, so the zone queue must be asked to fill it.
func TestRecordStopOutcomeFillsFreedSlot(t *testing.T) {
	id := uuid.NewString()
	outcome := &outcomeRepo{current: domain.Order{
		ID: id, Status: domain.StatusInTransit, RouteID: "zone-1#1", ZoneID: "zone-1",
		DeliveryDate: mustDeliveryDate(t, "2030-04-12"),
	}}
	zoneRepo := &fakeQueueRepo{}
	queue := NewZoneQueue(zoneRepo, &fakeCapacity{max: 2})

	if _, err := NewRecordStopOutcome(outcome).WithZoneQueue(queue).Run(context.Background(), RecordStopOutcomeCommand{
		Role: RoleDriver, RouteID: "zone-1#1", OrderID: id, Outcome: domain.StatusDelivered,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fillCall{"zone-1", "2030-04-12", 2}
	if len(zoneRepo.fillCalls) != 1 || zoneRepo.fillCalls[0] != want {
		t.Fatalf("fill calls = %+v, want %+v", zoneRepo.fillCalls, want)
	}
}

// Cancelling an order that held a route slot releases it; the queue must be filled.
func TestCancelOrderFillsReleasedSlot(t *testing.T) {
	id := uuid.NewString()
	orders := &queueCancelRepo{current: domain.Order{
		ID: id, Status: domain.StatusRouteAssigned, RouteID: "zone-1#1", ZoneID: "zone-1",
		DeliveryDate: mustDeliveryDate(t, "2030-04-12"),
	}}
	customers := &queueCustomers{customer: domain.Customer{ID: uuid.New()}}
	zoneRepo := &fakeQueueRepo{}

	if _, err := NewCancelOrder(customers, orders).WithZoneQueue(NewZoneQueue(zoneRepo, &fakeCapacity{max: 2})).Run(context.Background(), CancelCommand{
		UserID: uuid.New(), Role: RoleCustomer, OrderID: id,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(zoneRepo.fillCalls) != 1 {
		t.Fatalf("fill calls = %d, want 1", len(zoneRepo.fillCalls))
	}
}

// Reads attach the queue position so the customer can see "#N en espera".
func TestListMyOrdersAttachesQueuePosition(t *testing.T) {
	waiting := domain.Order{
		ID: uuid.NewString(), Status: domain.StatusPending, ZoneID: "zone-1",
		DeliveryDate: mustDeliveryDate(t, "2030-04-12"),
	}
	customers := &queueCustomers{customer: domain.Customer{ID: uuid.New()}}
	orders := &queueListRepo{orders: []domain.Order{waiting}}
	zoneRepo := &fakeQueueRepo{position: 2}

	got, err := NewListMyOrders(customers, orders).WithZoneQueue(NewZoneQueue(zoneRepo, &fakeCapacity{max: 2})).Run(context.Background(), Command{
		UserID: uuid.New(), Role: RoleCustomer,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 || got[0].QueuePosition != 2 {
		t.Fatalf("queue position = %+v, want 2", got)
	}
}
