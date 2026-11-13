package domain

import (
	"errors"
	"time"
)

var (
	ErrEmptyOrderEdit          = errors.New("at least one delivery field must change")
	ErrOrderEditState          = errors.New("the order status does not allow this change")
	ErrIncompleteDeliveryPoint = errors.New("address, latitude and longitude must be supplied together")
)

type CustomerOrderEdit struct {
	Address, DeliveryDate, WindowStart, WindowEnd *string
	Latitude, Longitude                           *float64
	AddressReference, PostalCode, ContactPhone    *string
}

func (e CustomerOrderEdit) HasPoint() bool {
	return e.Address != nil || e.Latitude != nil || e.Longitude != nil
}

func (o Order) EditDelivery(e CustomerOrderEdit, today time.Time) (Order, error) {
	if e == (CustomerOrderEdit{}) {
		return Order{}, ErrEmptyOrderEdit
	}
	switch o.Status {
	case StatusPending:
		if o.RouteID != "" {
			return Order{}, ErrOrderEditState
		}
	case StatusRouteAssigned:
		if e.HasPoint() || e.DeliveryDate != nil || e.WindowStart != nil || e.WindowEnd != nil {
			return Order{}, ErrOrderEditState
		}
	default:
		return Order{}, ErrOrderEditState
	}

	var err error
	if e.HasPoint() {
		if e.Address == nil || e.Latitude == nil || e.Longitude == nil {
			return Order{}, ErrIncompleteDeliveryPoint
		}
		o.Address, err = deliveryAddress(*e.Address, *e.Latitude, *e.Longitude)
		if err != nil {
			return Order{}, err
		}
		o.Latitude, o.Longitude = *e.Latitude, *e.Longitude
	}
	if e.DeliveryDate != nil {
		o.DeliveryDate, err = ParseDeliveryDate(*e.DeliveryDate)
		if err != nil {
			return Order{}, err
		}
		if o.DeliveryDate.BeforeLocalDay(today) {
			return Order{}, ErrPastDeliveryDate
		}
	}
	if e.WindowStart != nil || e.WindowEnd != nil {
		start, end := o.Window.Start(), o.Window.End()
		if e.WindowStart != nil {
			start = *e.WindowStart
		}
		if e.WindowEnd != nil {
			end = *e.WindowEnd
		}
		o.Window, err = NewDeliveryWindow(start, end)
		if err != nil {
			return Order{}, err
		}
	}
	if e.AddressReference != nil {
		o.AddressReference, err = addressReference(*e.AddressReference)
		if err != nil {
			return Order{}, err
		}
	}
	if e.PostalCode != nil {
		o.PostalCode, err = postalCode(*e.PostalCode)
		if err != nil {
			return Order{}, err
		}
	}
	if e.ContactPhone != nil {
		o.ContactPhone, err = NormalizePhone(*e.ContactPhone)
		if err != nil {
			return Order{}, err
		}
	}
	return o, nil
}
