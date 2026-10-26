// Package domain contains order rules without transport or persistence dependencies.
package domain

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Order keeps the registration instant separate from the planned delivery day.
// ID and version are assigned by persistence in the next sprint.
type Order struct {
	ID         string
	CustomerID string
	ZoneID     string
	// RouteID is set when the order is assigned to a route (ROUTE_ASSIGNED).
	RouteID string
	// StopSequence is the order's position in RouteID (1 is the first stop). 0 means unassigned:
	// the order was assigned through the legacy form without a sequence, or it has no route.
	StopSequence int

	Address      string
	Latitude     float64
	Longitude    float64
	DeliveryDate DeliveryDate
	Window       DeliveryWindow
	// AddressReference, PostalCode and ContactPhone are empty only in orders created before they existed.
	AddressReference string
	PostalCode       string
	ContactPhone     string
	Status           Status
	// ReviewReasons is non-empty only while Status is PENDING_REVIEW (ADR 0007).
	ReviewReasons []ReviewReason
	CreatedAt     time.Time
	Version       int64
	// QueuePosition is the FIFO place (1 is next) of a PENDING order waiting for a free route slot in
	// its zone. 0 when the order is not waiting. It is computed by the application layer for reads
	// and is never persisted.
	QueuePosition int
}

// NewOrderInput contains only caller-controlled delivery data.
type NewOrderInput struct {
	Address      string
	Latitude     float64
	Longitude    float64
	DeliveryDate string
	WindowStart  string
	WindowEnd    string
	// AddressReference and ContactPhone are required; PostalCode is optional.
	AddressReference string
	PostalCode       string
	ContactPhone     string
}

// NewOrder receives the clock instant in the operating timezone. This makes
// the local-day rule deterministic, including around midnight. CreatedAt is
// stored in UTC, but deliveryDate is preserved as the selected calendar date.
func NewOrder(input NewOrderInput, registeredAt time.Time) (Order, error) {
	if registeredAt.IsZero() || registeredAt.Year() < 1 || registeredAt.Year() > 9999 {
		return Order{}, ErrInvalidCreatedAt
	}
	address := strings.TrimSpace(input.Address)
	length := utf8.RuneCountInString(address)
	if !utf8.ValidString(address) || strings.ContainsRune(address, '\x00') || length < 5 || length > 300 {
		return Order{}, ErrInvalidAddress
	}
	if !validCoordinates(input.Latitude, input.Longitude) {
		return Order{}, ErrInvalidCoordinate
	}
	date, err := ParseDeliveryDate(input.DeliveryDate)
	if err != nil {
		return Order{}, err
	}
	if date.BeforeLocalDay(registeredAt) {
		return Order{}, ErrPastDeliveryDate
	}
	window, err := NewDeliveryWindow(input.WindowStart, input.WindowEnd)
	if err != nil {
		return Order{}, err
	}
	reference := strings.TrimSpace(input.AddressReference)
	if n := utf8.RuneCountInString(reference); !utf8.ValidString(reference) || strings.ContainsRune(reference, '\x00') || n < 3 || n > 200 {
		return Order{}, ErrInvalidAddressReference
	}
	postalCode := strings.TrimSpace(input.PostalCode)
	if postalCode != "" && !validPostalCode(postalCode) {
		return Order{}, ErrInvalidPostalCode
	}
	phone, err := NormalizePhone(input.ContactPhone)
	if err != nil {
		return Order{}, err
	}
	return Order{
		Address: address, Latitude: input.Latitude, Longitude: input.Longitude,
		DeliveryDate: date, Window: window, Status: StatusPending,
		AddressReference: reference, PostalCode: postalCode, ContactPhone: phone,
		CreatedAt: registeredAt.UTC(),
	}, nil
}

func validPostalCode(code string) bool {
	if len(code) < 4 || len(code) > 10 {
		return false
	}
	for _, r := range code {
		if !isPostalCodeChar(r) {
			return false
		}
	}
	return true
}

// isPostalCodeChar accepts the letters, digits and hyphen allowed in a postal code.
func isPostalCodeChar(r rune) bool {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
		return true
	default:
		return false
	}
}

// TransitionTo returns an updated copy; a rejected transition leaves the order intact.
func (o Order) TransitionTo(next Status) (Order, error) {
	if _, err := ParseStatus(string(next)); err != nil {
		return Order{}, err
	}
	if !o.Status.CanTransitionTo(next) {
		return Order{}, ErrInvalidTransition
	}
	o.Status = next
	return o, nil
}

func validCoordinates(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsNaN(longitude) &&
		!math.IsInf(latitude, 0) && !math.IsInf(longitude, 0) &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}
