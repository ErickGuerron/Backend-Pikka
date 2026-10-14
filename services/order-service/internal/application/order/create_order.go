// Package order contiene los casos de uso del dominio Order.
package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RoleCustomer es el único rol que puede registrar pedidos (ADR 0007).
const RoleCustomer = "CUSTOMER"

// Command es la entrada del alta de pedido. La identidad viene de la metadata del
// Gateway, nunca del cuerpo. Las coordenadas son opcionales: si faltan, el backend
// geocodifica la dirección.
type Command struct {
	UserID       uuid.UUID
	Role         string
	Address      string
	Latitude     *float64
	Longitude    *float64
	DeliveryDate string
	WindowStart  string
	WindowEnd    string
}

// CreateOrder registra un pedido con validación automática (ADR 0007): la dirección
// se localiza, la zona la cubre y el cliente no está bloqueado. Si algo no cumple,
// el pedido queda en PENDING_REVIEW y el cliente recibe un aviso.
type CreateOrder struct {
	customers ports.CustomerRepository
	orders    ports.OrderRepository
	place     placement
	location  *time.Location
	clock     func() time.Time
}

// NewCreateOrder inyecta dependencias. location es la zona horaria operativa.
func NewCreateOrder(customers ports.CustomerRepository, zones ports.ZoneClassifier, geocoder ports.Geocoder, orders ports.OrderRepository, location *time.Location) *CreateOrder {
	return &CreateOrder{
		customers: customers,
		orders:    orders,
		place:     placement{zones: zones, geocoder: geocoder},
		location:  location,
		clock:     time.Now,
	}
}

// Run valida el pedido y lo persiste.
func (uc *CreateOrder) Run(ctx context.Context, cmd Command) (domain.Order, error) {
	if cmd.Role != RoleCustomer {
		return domain.Order{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return domain.Order{}, err
	}

	var known *coordinates
	if cmd.Latitude != nil && cmd.Longitude != nil {
		known = &coordinates{latitude: *cmd.Latitude, longitude: *cmd.Longitude}
	}
	point, located, err := uc.place.locate(ctx, cmd.Address, known)
	if err != nil {
		return domain.Order{}, err
	}

	order, err := domain.NewOrder(domain.NewOrderInput{
		Address:      cmd.Address,
		Latitude:     point.latitude,
		Longitude:    point.longitude,
		DeliveryDate: cmd.DeliveryDate,
		WindowStart:  cmd.WindowStart,
		WindowEnd:    cmd.WindowEnd,
	}, uc.clock().In(uc.location))
	if err != nil {
		return domain.Order{}, err
	}

	reasons, zoneID, err := uc.place.reasons(ctx, point, located, !customer.CanCreateOrders())
	if err != nil {
		return domain.Order{}, err
	}
	order.CustomerID = customer.ID.String()
	order.ZoneID = zoneID
	order, err = order.ApplyValidation(reasons)
	if err != nil {
		return domain.Order{}, err
	}

	var notice *domain.Notification
	if len(reasons) > 0 {
		n := domain.NewNotification(customer.ID.String(), "", domain.NotificationInReview, reviewDetail(reasons))
		notice = &n
	}
	return uc.orders.Create(ctx, order, notice)
}
