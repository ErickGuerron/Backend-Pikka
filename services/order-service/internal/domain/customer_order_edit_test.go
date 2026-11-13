package domain

import (
	"errors"
	"testing"
	"time"
)

func TestCustomerDeliveryEditUsesStatusAndFieldRules(t *testing.T) {
	today := time.Date(2030, 4, 10, 9, 0, 0, 0, time.UTC)
	date, err := ParseDeliveryDate("2030-04-12")
	if err != nil {
		t.Fatal(err)
	}
	window, err := NewDeliveryWindow("09:00", "12:00")
	if err != nil {
		t.Fatal(err)
	}
	initial := Order{Status: StatusPending, DeliveryDate: date, Window: window, CreatedAt: today, Address: "Avenida anterior 100"}
	address, lat, lon := "Avenida nueva 123", -0.2, -78.5
	newDate, newStart, newEnd := "2030-04-13", "12:00", "15:00"
	phone, reference, postal := "(02) 123-4567", "Frente al parque", "170150"
	contact := CustomerOrderEdit{AddressReference: &reference, PostalCode: &postal, ContactPhone: &phone}
	full := contact
	full.Address, full.Latitude, full.Longitude = &address, &lat, &lon
	full.DeliveryDate, full.WindowStart, full.WindowEnd = &newDate, &newStart, &newEnd

	updated, err := initial.EditDelivery(full, today)
	if err != nil || updated.Address != address || updated.Latitude != lat || updated.Longitude != lon ||
		updated.DeliveryDate.String() != newDate || updated.Window.Start() != newStart || updated.Window.End() != newEnd ||
		updated.AddressReference != reference || updated.PostalCode != postal || updated.ContactPhone != "021234567" ||
		updated.CreatedAt != today {
		t.Fatalf("edited order = %+v, error = %v", updated, err)
	}

	assigned := initial
	assigned.Status, assigned.RouteID = StatusRouteAssigned, "route-a"
	updated, err = assigned.EditDelivery(contact, today)
	if err != nil || updated.RouteID != assigned.RouteID || updated.Address != initial.Address ||
		updated.PostalCode != postal || updated.ContactPhone != "021234567" || updated.AddressReference != reference {
		t.Fatalf("assigned edit = %+v, error = %v", updated, err)
	}
	past, badEnd, blank := "2030-04-09", "08:00", ""
	for _, tt := range []struct {
		name   string
		status Status
		edit   CustomerOrderEdit
		want   error
	}{
		{"no changes", StatusPending, CustomerOrderEdit{}, ErrEmptyOrderEdit},
		{"incomplete point", StatusPending, CustomerOrderEdit{Address: &address}, ErrIncompleteDeliveryPoint},
		{"past date", StatusPending, CustomerOrderEdit{DeliveryDate: &past}, ErrPastDeliveryDate},
		{"window ends before start", StatusPending, CustomerOrderEdit{WindowEnd: &badEnd}, ErrInvalidWindow},
		{"blank reference", StatusPending, CustomerOrderEdit{AddressReference: &blank}, ErrInvalidAddressReference},
		{"blank phone", StatusPending, CustomerOrderEdit{ContactPhone: &blank}, ErrInvalidPhone},
		{"assigned point", StatusRouteAssigned, CustomerOrderEdit{Address: &address}, ErrOrderEditState},
		{"assigned date", StatusRouteAssigned, CustomerOrderEdit{DeliveryDate: &newDate}, ErrOrderEditState},
		{"assigned window", StatusRouteAssigned, CustomerOrderEdit{WindowStart: &newStart}, ErrOrderEditState},
		{"in review", StatusPendingReview, contact, ErrOrderEditState},
		{"picked up", StatusPickedUp, contact, ErrOrderEditState},
		{"in transit", StatusInTransit, contact, ErrOrderEditState},
		{"delivered", StatusDelivered, contact, ErrOrderEditState},
		{"cancelled", StatusCancelled, contact, ErrOrderEditState},
	} {
		t.Run(tt.name, func(t *testing.T) {
			order := initial
			order.Status = tt.status
			if _, err := order.EditDelivery(tt.edit, today); !errors.Is(err, tt.want) {
				t.Fatalf("edit = %v, want %v", err, tt.want)
			}
		})
	}
}
