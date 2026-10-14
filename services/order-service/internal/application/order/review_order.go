package order

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Roles que revisan la cola de excepciones (ADR 0008).
const (
	RoleAdmin    = "ADMIN"
	RoleOperator = "OPERATOR"
)

// Acciones del operador sobre un pedido en revisión (ADR 0007, section 3).
const (
	ActionConfirm = "CONFIRM"
	ActionCorrect = "CORRECT"
	ActionReject  = "REJECT"
)

var (
	// ErrUnknownAction: la acción no pertenece al conjunto cerrado.
	ErrUnknownAction = errors.New("unknown review action")

	// ErrNothingToCorrect: CORRECT sin ningún campo que corregir.
	ErrNothingToCorrect = errors.New("correction must change at least one field")
)

// Correction son los campos que el operador ajusta. nil significa "sin cambio".
type Correction struct {
	Address      *string
	DeliveryDate *string
	WindowStart  *string
	WindowEnd    *string
}

func (c *Correction) empty() bool {
	return c == nil || (c.Address == nil && c.DeliveryDate == nil && c.WindowStart == nil && c.WindowEnd == nil)
}

// ReviewCommand es una decisión del operador sobre un pedido en revisión.
type ReviewCommand struct {
	Role       string
	OrderID    string
	Action     string
	Reason     string
	Correction *Correction
}

// ReviewOrder aplica la decisión del operador. Todo queda con motivo y el cliente
// recibe un aviso en la misma transacción.
type ReviewOrder struct {
	customers ports.CustomerRepository
	orders    ports.OrderRepository
	place     placement
	location  *time.Location
	clock     func() time.Time
}

// NewReviewOrder inyecta dependencias.
func NewReviewOrder(customers ports.CustomerRepository, zones ports.ZoneClassifier, geocoder ports.Geocoder, orders ports.OrderRepository, location *time.Location) *ReviewOrder {
	return &ReviewOrder{
		customers: customers,
		orders:    orders,
		place:     placement{zones: zones, geocoder: geocoder},
		location:  location,
		clock:     time.Now,
	}
}

// Run valida la acción y la aplica dentro de la transacción del repositorio.
func (uc *ReviewOrder) Run(ctx context.Context, cmd ReviewCommand) (domain.Order, error) {
	if !isOperator(cmd.Role) {
		return domain.Order{}, domain.ErrNotOperator
	}
	reason, err := domain.ReviewDecisionReason(cmd.Reason)
	if err != nil {
		return domain.Order{}, err
	}
	decide := func(current domain.Order) (domain.Order, *domain.Notification, error) {
		switch cmd.Action {
		case ActionConfirm:
			next, err := current.Confirm()
			if err != nil {
				return domain.Order{}, nil, err
			}
			n := domain.NewNotification(current.CustomerID, current.ID, domain.NotificationConfirmed, reason)
			return next, &n, nil
		case ActionReject:
			next, err := current.Reject()
			if err != nil {
				return domain.Order{}, nil, err
			}
			n := domain.NewNotification(current.CustomerID, current.ID, domain.NotificationRejected, reason)
			return next, &n, nil
		case ActionCorrect:
			return uc.correct(ctx, current, cmd.Correction, reason)
		default:
			return domain.Order{}, nil, ErrUnknownAction
		}
	}
	return uc.orders.Review(ctx, cmd.OrderID, decide)
}

// correct vuelve a validar el pedido con los datos corregidos. Si ya cumple las
// reglas queda PENDING; si no, sigue en revisión con los motivos actualizados.
func (uc *ReviewOrder) correct(ctx context.Context, current domain.Order, corr *Correction, reason string) (domain.Order, *domain.Notification, error) {
	if corr.empty() {
		return domain.Order{}, nil, ErrNothingToCorrect
	}
	address := current.Address
	addressChanged := false
	if corr.Address != nil {
		address = strings.TrimSpace(*corr.Address)
		addressChanged = address != current.Address
	}
	date := current.DeliveryDate.String()
	if corr.DeliveryDate != nil {
		date = *corr.DeliveryDate
	}
	start, end := current.Window.Start(), current.Window.End()
	if corr.WindowStart != nil {
		start = *corr.WindowStart
	}
	if corr.WindowEnd != nil {
		end = *corr.WindowEnd
	}

	// Las coordenadas actuales se reutilizan si la dirección no cambió y ya estaba localizada.
	var known *coordinates
	if !addressChanged && !hasReason(current.ReviewReasons, domain.ReviewAddressNotLocated) {
		known = &coordinates{latitude: current.Latitude, longitude: current.Longitude}
	}
	point, located, err := uc.place.locate(ctx, address, known)
	if err != nil {
		return domain.Order{}, nil, err
	}

	next, err := domain.NewOrder(domain.NewOrderInput{
		Address:      address,
		Latitude:     point.latitude,
		Longitude:    point.longitude,
		DeliveryDate: date,
		WindowStart:  start,
		WindowEnd:    end,
	}, uc.clock().In(uc.location))
	if err != nil {
		return domain.Order{}, nil, err
	}

	customerID, err := uuid.Parse(current.CustomerID)
	if err != nil {
		return domain.Order{}, nil, err
	}
	customer, err := uc.customers.GetByID(ctx, customerID)
	if err != nil {
		return domain.Order{}, nil, err
	}
	reasons, zoneID, err := uc.place.reasons(ctx, point, located, !customer.CanCreateOrders())
	if err != nil {
		return domain.Order{}, nil, err
	}

	next.ID = current.ID
	next.CustomerID = current.CustomerID
	next.CreatedAt = current.CreatedAt
	next.Version = current.Version
	next.ZoneID = zoneID
	next, err = next.ApplyValidation(reasons)
	if err != nil {
		return domain.Order{}, nil, err
	}

	if len(reasons) == 0 {
		n := domain.NewNotification(current.CustomerID, current.ID, domain.NotificationCorrected, reason)
		return next, &n, nil
	}
	n := domain.NewNotification(current.CustomerID, current.ID, domain.NotificationInReview, reviewDetail(reasons))
	return next, &n, nil
}

// ListExceptions devuelve la cola de excepciones para operadores.
type ListExceptions struct {
	orders ports.OrderRepository
}

// NewListExceptions inyecta el repositorio.
func NewListExceptions(orders ports.OrderRepository) *ListExceptions {
	return &ListExceptions{orders: orders}
}

// Run devuelve los pedidos en revisión, del más antiguo al más reciente.
func (uc *ListExceptions) Run(ctx context.Context, role string) ([]domain.Order, error) {
	if !isOperator(role) {
		return nil, domain.ErrNotOperator
	}
	return uc.orders.ListExceptions(ctx)
}

func isOperator(role string) bool {
	return role == RoleAdmin || role == RoleOperator
}

func hasReason(reasons []domain.ReviewReason, want domain.ReviewReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
