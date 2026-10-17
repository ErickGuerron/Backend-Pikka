package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// getOrderRepo aplica el mismo criterio de propiedad que la consulta SQL (WHERE id AND customer_id)
// y registra con qué cliente se pidió la lectura.
type getOrderRepo struct {
	ports.OrderRepository
	orders     map[string]domain.Order
	calledWith uuid.UUID
	calls      int
}

func (r *getOrderRepo) GetForCustomer(_ context.Context, orderID string, customerID uuid.UUID) (domain.Order, error) {
	r.calls++
	r.calledWith = customerID
	order, ok := r.orders[orderID]
	if !ok || order.CustomerID != customerID.String() {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

// getMyOrderFixture devuelve el caso de uso con un pedido propio que tiene ruta asignada.
func getMyOrderFixture(t *testing.T) (*GetMyOrder, *getOrderRepo, domain.Order) {
	t.Helper()
	customer := activeCustomer()
	order := domain.Order{
		ID:         uuid.NewString(),
		CustomerID: customer.ID.String(),
		RouteID:    uuid.NewString(),
		Status:     domain.StatusRouteAssigned,
	}
	repo := &getOrderRepo{orders: map[string]domain.Order{order.ID: order}}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	return NewGetMyOrder(customers, repo), repo, order
}

func TestGetMyOrderCustomerReadsOwnOrderWithRouteID(t *testing.T) {
	uc, repo, order := getMyOrderFixture(t)

	got, err := uc.Run(context.Background(), GetCommand{UserID: customerUser, Role: RoleCustomer, OrderID: order.ID})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.ID != order.ID {
		t.Fatalf("id = %s, want %s", got.ID, order.ID)
	}
	if got.RouteID != order.RouteID {
		t.Fatalf("route id = %q, want %q", got.RouteID, order.RouteID)
	}
	if repo.calledWith != customerID {
		t.Fatalf("repository got customer %s, want the authenticated customer", repo.calledWith)
	}
}

func TestGetMyOrderOfAnotherCustomerIsNotFound(t *testing.T) {
	uc, _, order := getMyOrderFixture(t)
	other := domain.Customer{ID: uuid.New(), UserID: uuid.New(), Status: domain.StatusActive}
	uc.customers = &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{other.UserID: other}}

	_, err := uc.Run(context.Background(), GetCommand{UserID: other.UserID, Role: RoleCustomer, OrderID: order.ID})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound (BOLA: no 403, no leak)", err)
	}
}

func TestGetMyOrderRejectsNonCustomerRole(t *testing.T) {
	uc, repo, order := getMyOrderFixture(t)

	_, err := uc.Run(context.Background(), GetCommand{UserID: customerUser, Role: RoleOperator, OrderID: order.ID})

	if !errors.Is(err, domain.ErrNotCustomer) {
		t.Fatalf("err = %v, want ErrNotCustomer", err)
	}
	if repo.calls != 0 {
		t.Fatal("a non-customer must not reach the repository")
	}
}

func TestGetMyOrderUnknownIDIsNotFound(t *testing.T) {
	uc, _, _ := getMyOrderFixture(t)

	_, err := uc.Run(context.Background(), GetCommand{UserID: customerUser, Role: RoleCustomer, OrderID: uuid.NewString()})

	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
}
