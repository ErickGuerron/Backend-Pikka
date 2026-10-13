package domain_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func validInput() domain.NewOrderInput {
	return domain.NewOrderInput{
		Address: "  Avenida Amazonas 123  ", Latitude: -0.18, Longitude: -78.48,
		DeliveryDate: "2026-10-25", WindowStart: "09:00", WindowEnd: "12:00",
	}
}

func registrationTime() time.Time {
	return time.Date(2026, 10, 11, 23, 30, 0, 0, time.FixedZone("operating", -5*60*60))
}

func TestNewOrderSeparatesRegistrationInstantAndDeliveryDate(t *testing.T) {
	order, err := domain.NewOrder(validInput(), registrationTime())
	if err != nil {
		t.Fatal(err)
	}
	if order.Address != "Avenida Amazonas 123" || order.Status != domain.StatusPending {
		t.Fatalf("unexpected initial order: %+v", order)
	}
	if order.DeliveryDate.String() != "2026-10-25" || order.CreatedAt.Format(time.RFC3339) != "2026-10-12T04:30:00Z" {
		t.Fatalf("registration and delivery dates were conflated: %+v", order)
	}
	if order.Window.Start() != "09:00" || order.Window.End() != "12:00" {
		t.Fatalf("delivery window changed: %+v", order.Window)
	}
	if order.ID != "" || order.Version != 0 {
		t.Fatal("persistence must assign ID and version")
	}
}

func TestNewOrderUsesLocalDayAtUTCMidnight(t *testing.T) {
	for _, offset := range []int{-5, 14} {
		t.Run(fmt.Sprintf("UTC%+d", offset), func(t *testing.T) {
			instant := time.Date(2026, 10, 11, 0, 30, 0, 0, time.FixedZone("operating", offset*60*60))
			if offset < 0 {
				instant = instant.Add(23 * time.Hour)
			}
			input := validInput()
			input.DeliveryDate = "2026-10-11"
			if _, err := domain.NewOrder(input, instant); err != nil {
				t.Fatalf("local today must be accepted even when UTC day differs: %v", err)
			}
			input.DeliveryDate = "2026-10-10"
			if _, err := domain.NewOrder(input, instant); !errors.Is(err, domain.ErrPastDeliveryDate) {
				t.Fatalf("local yesterday: got %v", err)
			}
		})
	}
}

func TestNewOrderRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		change func(*domain.NewOrderInput)
		want   error
	}{
		{"blank address", func(i *domain.NewOrderInput) { i.Address = " \t\n " }, domain.ErrInvalidAddress},
		{"short address", func(i *domain.NewOrderInput) { i.Address = "1234" }, domain.ErrInvalidAddress},
		{"long address", func(i *domain.NewOrderInput) { i.Address = strings.Repeat("á", 301) }, domain.ErrInvalidAddress},
		{"invalid UTF8", func(i *domain.NewOrderInput) { i.Address = "abcde\xff" }, domain.ErrInvalidAddress},
		{"NUL in address", func(i *domain.NewOrderInput) { i.Address = "Avenida\x00Amazonas 123" }, domain.ErrInvalidAddress},
		{"latitude above range", func(i *domain.NewOrderInput) { i.Latitude = 90.01 }, domain.ErrInvalidCoordinate},
		{"latitude below range", func(i *domain.NewOrderInput) { i.Latitude = -90.01 }, domain.ErrInvalidCoordinate},
		{"longitude above range", func(i *domain.NewOrderInput) { i.Longitude = 180.01 }, domain.ErrInvalidCoordinate},
		{"longitude below range", func(i *domain.NewOrderInput) { i.Longitude = -180.01 }, domain.ErrInvalidCoordinate},
		{"NaN latitude", func(i *domain.NewOrderInput) { i.Latitude = math.NaN() }, domain.ErrInvalidCoordinate},
		{"NaN longitude", func(i *domain.NewOrderInput) { i.Longitude = math.NaN() }, domain.ErrInvalidCoordinate},
		{"infinite latitude", func(i *domain.NewOrderInput) { i.Latitude = math.Inf(1) }, domain.ErrInvalidCoordinate},
		{"infinite longitude", func(i *domain.NewOrderInput) { i.Longitude = math.Inf(-1) }, domain.ErrInvalidCoordinate},
		{"invalid date", func(i *domain.NewOrderInput) { i.DeliveryDate = "2026-02-30" }, domain.ErrInvalidDate},
		{"date with time", func(i *domain.NewOrderInput) { i.DeliveryDate = "2026-10-25T00:00:00Z" }, domain.ErrInvalidDate},
		{"past date", func(i *domain.NewOrderInput) { i.DeliveryDate = "2026-10-10" }, domain.ErrPastDeliveryDate},
		{"equal window", func(i *domain.NewOrderInput) { i.WindowEnd = i.WindowStart }, domain.ErrInvalidWindow},
		{"reversed window", func(i *domain.NewOrderInput) { i.WindowEnd = "08:00" }, domain.ErrInvalidWindow},
		{"overnight window", func(i *domain.NewOrderInput) { i.WindowStart = "23:00"; i.WindowEnd = "01:00" }, domain.ErrInvalidWindow},
		{"non padded hour", func(i *domain.NewOrderInput) { i.WindowStart = "9:00" }, domain.ErrInvalidWindow},
		{"24 hour clock", func(i *domain.NewOrderInput) { i.WindowEnd = "24:00" }, domain.ErrInvalidWindow},
		{"seconds", func(i *domain.NewOrderInput) { i.WindowEnd = "12:00:01" }, domain.ErrInvalidWindow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validInput()
			tt.change(&input)
			if _, err := domain.NewOrder(input, registrationTime()); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewOrderAcceptsUnicodeAndCoordinateBoundaries(t *testing.T) {
	for _, length := range []int{5, 300} {
		input := validInput()
		input.Address = strings.Repeat("ñ", length)
		for _, latitude := range []float64{-90, 0, 90} {
			for _, longitude := range []float64{-180, 0, 180} {
				input.Latitude, input.Longitude = latitude, longitude
				if _, err := domain.NewOrder(input, registrationTime()); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestNewOrderRequiresServerClock(t *testing.T) {
	if _, err := domain.NewOrder(validInput(), time.Time{}); !errors.Is(err, domain.ErrInvalidCreatedAt) {
		t.Fatalf("zero clock: %v", err)
	}
}

func TestDeliveryDateValidatesCalendarAndPreservesDate(t *testing.T) {
	for _, value := range []string{"0001-01-01", "2028-02-29", "9999-12-31"} {
		date, err := domain.ParseDeliveryDate(value)
		if err != nil || date.String() != value {
			t.Fatalf("%s: date=%s err=%v", value, date.String(), err)
		}
	}
	for _, value := range []string{"", "0000-01-01", "2026-02-29", "2026-13-01", "2026-2-01", "2026-01-1", " 2026-10-25"} {
		if _, err := domain.ParseDeliveryDate(value); !errors.Is(err, domain.ErrInvalidDate) {
			t.Fatalf("accepted invalid date %q: %v", value, err)
		}
	}
}

func TestDeliveryWindowBoundaries(t *testing.T) {
	window, err := domain.NewDeliveryWindow("00:00", "23:59")
	if err != nil || window.Start() != "00:00" || window.End() != "23:59" {
		t.Fatalf("valid full-day window rejected: %v", err)
	}
	for _, pair := range [][2]string{{"", "10:00"}, {"09:60", "10:00"}, {"-1:00", "10:00"}, {"09:00", "12:60"}} {
		if _, err := domain.NewDeliveryWindow(pair[0], pair[1]); !errors.Is(err, domain.ErrInvalidWindow) {
			t.Fatalf("accepted invalid window %v: %v", pair, err)
		}
	}
}

func TestStatusTransitionsAndCancellation(t *testing.T) {
	states := []domain.Status{
		domain.StatusPending, domain.StatusPendingReview, domain.StatusRouteAssigned,
		domain.StatusPickedUp, domain.StatusInTransit, domain.StatusDelivered,
		domain.StatusNotDelivered, domain.StatusCancelled,
	}
	allowed := map[[2]domain.Status]bool{
		{domain.StatusPendingReview, domain.StatusPending}:   true,
		{domain.StatusPendingReview, domain.StatusCancelled}: true,
		{domain.StatusPending, domain.StatusRouteAssigned}:   true,
		{domain.StatusPending, domain.StatusCancelled}:       true,
		{domain.StatusRouteAssigned, domain.StatusPickedUp}:  true,
		{domain.StatusRouteAssigned, domain.StatusCancelled}: true,
		{domain.StatusPickedUp, domain.StatusInTransit}:      true,
		{domain.StatusInTransit, domain.StatusDelivered}:     true,
		{domain.StatusInTransit, domain.StatusNotDelivered}:  true,
	}
	for _, from := range states {
		parsed, err := domain.ParseStatus(string(from))
		if err != nil || parsed != from {
			t.Fatalf("parse %s: %v", from, err)
		}
		canCancel := from == domain.StatusPending || from == domain.StatusRouteAssigned
		if from.CanCancel() != canCancel {
			t.Fatalf("incorrect cancellation policy for %s", from)
		}
		for _, to := range states {
			if got, want := from.CanTransitionTo(to), allowed[[2]domain.Status{from, to}]; got != want {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, want)
			}
		}
	}
	for _, value := range []string{"", "pending", "UNKNOWN", "ORDER_STATUS_PENDING"} {
		if _, err := domain.ParseStatus(value); !errors.Is(err, domain.ErrInvalidOrderStatus) {
			t.Errorf("unknown status %q: %v", value, err)
		}
	}
}

func TestTransitionPreservesDeliveryDataAndRejectsInvalidChanges(t *testing.T) {
	order, err := domain.NewOrder(validInput(), registrationTime())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := order.TransitionTo(domain.StatusRouteAssigned)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.StatusRouteAssigned || order.Status != domain.StatusPending ||
		updated.CreatedAt != order.CreatedAt || updated.DeliveryDate != order.DeliveryDate || updated.Window != order.Window {
		t.Fatal("transition changed original order or delivery schedule")
	}
	if _, err := order.TransitionTo(domain.StatusDelivered); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("skipped steps: %v", err)
	}
	if _, err := order.TransitionTo("UNKNOWN"); !errors.Is(err, domain.ErrInvalidOrderStatus) {
		t.Fatalf("unknown next status: %v", err)
	}
	invalid := domain.Order{}
	if _, err := invalid.TransitionTo(domain.StatusPending); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("zero order: %v", err)
	}
}
