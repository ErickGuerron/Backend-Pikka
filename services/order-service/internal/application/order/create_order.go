// Package order contiene los casos de uso del dominio Order.
package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

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
	// AddressReference y ContactPhone son obligatorios; PostalCode es opcional.
	AddressReference string
	PostalCode       string
	ContactPhone     string
	// Items son los productos del pedido (ADR 0009). Al menos uno.
	Items []LineRequest
}

// LineRequest es un producto pedido y su cantidad. El precio lo toma el servidor del catálogo.
type LineRequest struct {
	ProductID uuid.UUID
	Quantity  int64
}

// CreateOrder registra un pedido con validación automática (ADR 0007): la dirección
// se localiza, la zona la cubre y el cliente no está bloqueado. Si algo no cumple,
// el pedido queda en PENDING_REVIEW y el cliente recibe un aviso. El pedido, sus productos
// y el descuento de stock se guardan juntos (ADR 0009).
type CreateOrder struct {
	customers ports.CustomerRepository
	products  ports.CatalogRepository
	placer    ports.OrderPlacer
	place     placement
	location  *time.Location
	clock     func() time.Time
	queue     *ZoneQueue
}

// WithZoneQueue hace que un pedido que no encuentra cupo libre en su zona quede en la cola de espera
// y tome el primer cupo que se libere (ver ZoneQueue).
func (uc *CreateOrder) WithZoneQueue(queue *ZoneQueue) *CreateOrder {
	uc.queue = queue
	return uc
}

// NewCreateOrder inyecta dependencias. location es la zona horaria operativa.
func NewCreateOrder(customers ports.CustomerRepository, zones ports.ZoneClassifier, geocoder ports.Geocoder, products ports.CatalogRepository, placer ports.OrderPlacer, location *time.Location) *CreateOrder {
	return &CreateOrder{
		customers: customers,
		products:  products,
		placer:    placer,
		place:     placement{zones: zones, geocoder: geocoder},
		location:  location,
		clock:     time.Now,
	}
}

// Run valida el pedido y lo persiste.
func (uc *CreateOrder) Place(ctx context.Context, cmd Command) (Placed, error) {
	if cmd.Role != RoleCustomer {
		return Placed{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return Placed{}, err
	}
	// ADR 0006, section 5: an account blocked for an unpaid balance cannot create orders. It is rejected
	// before anything else, so nothing is geocoded, reserved or stored. A block for incidents still goes
	// to the operator's review queue (ADR 0007, section 3).
	if customer.Status == domain.StatusBlockedSurcharge {
		return Placed{}, domain.ErrCustomerBlockedBySurcharge
	}
	lines, err := uc.resolveLines(ctx, cmd.Items)
	if err != nil {
		return Placed{}, err
	}

	var known *coordinates
	if cmd.Latitude != nil && cmd.Longitude != nil {
		known = &coordinates{latitude: *cmd.Latitude, longitude: *cmd.Longitude}
	}
	point, located, err := uc.place.locate(ctx, cmd.Address, known)
	if err != nil {
		return Placed{}, err
	}

	order, err := domain.NewOrder(domain.NewOrderInput{
		Address:      cmd.Address,
		Latitude:     point.latitude,
		Longitude:    point.longitude,
		DeliveryDate: cmd.DeliveryDate,
		WindowStart:  cmd.WindowStart,
		WindowEnd:    cmd.WindowEnd,

		AddressReference: cmd.AddressReference,
		PostalCode:       cmd.PostalCode,
		ContactPhone:     cmd.ContactPhone,
	}, uc.clock().In(uc.location))
	if err != nil {
		return Placed{}, err
	}

	reasons, zoneID, err := uc.place.reasons(ctx, point, located, !customer.CanCreateOrders())
	if err != nil {
		return Placed{}, err
	}
	order.CustomerID = customer.ID.String()
	order.ZoneID = zoneID
	order, err = order.ApplyValidation(reasons)
	if err != nil {
		return Placed{}, err
	}

	var notice *domain.Notification
	if len(reasons) > 0 {
		n := domain.NewNotification(customer.ID.String(), "", domain.NotificationInReview, reviewDetail(reasons))
		notice = &n
	}
	actor := cmd.UserID
	saved, err := uc.placer.PlaceOrder(ctx, order, lines, notice, &actor)
	if err != nil {
		return Placed{}, err
	}
	if saved.Status == domain.StatusPending {
		uc.queue.fillNonFatal(ctx, saved.ZoneID, saved.DeliveryDate.String())
		// The fill may have routed the order already; the stored state decides its place in line.
		if position, err := uc.queue.positionOf(ctx, saved); err == nil {
			saved.QueuePosition = position
		}
	}
	total, err := domain.OrderTotal(lines)
	if err != nil {
		return Placed{}, err
	}
	return Placed{Order: saved, Lines: lines, Total: total, PendingSurcharge: customer.PendingSurcharge}, nil
}

// Run registra el pedido y devuelve solo el pedido guardado. Place devuelve además sus productos y total.
func (uc *CreateOrder) Run(ctx context.Context, cmd Command) (domain.Order, error) {
	placed, err := uc.Place(ctx, cmd)
	return placed.Order, err
}

// Placed es el pedido guardado con sus productos y su total, para la respuesta del alta.
type Placed struct {
	Order domain.Order
	Lines []domain.OrderLine
	Total decimal.Decimal
	// PendingSurcharge is what the customer owes when the order is confirmed (ADR 0006, section 4). It is a
	// separate line: it is never added to Total.
	PendingSurcharge decimal.Decimal
}

// resolveLines toma cada producto del catálogo y valida la cantidad y que esté activo. Un pedido
// sin productos o con un producto repetido no se acepta.
func (uc *CreateOrder) resolveLines(ctx context.Context, items []LineRequest) ([]domain.OrderLine, error) {
	if len(items) == 0 {
		return nil, domain.ErrNoItems
	}
	seen := make(map[uuid.UUID]bool, len(items))
	lines := make([]domain.OrderLine, 0, len(items))
	for _, item := range items {
		if seen[item.ProductID] {
			return nil, domain.ErrDuplicateLine
		}
		seen[item.ProductID] = true
		product, err := uc.products.GetProduct(ctx, item.ProductID)
		if err != nil {
			return nil, err
		}
		line, err := domain.NewOrderLine(product, item.Quantity)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}
