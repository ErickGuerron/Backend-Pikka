package domain

import "time"

// DeliveryDate is a civil date, independent of UTC offsets and registration time.
// Its zero value is invalid. Construct it with ParseDeliveryDate.
type DeliveryDate struct {
	date string
}

func ParseDeliveryDate(value string) (DeliveryDate, error) {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil || date.Format(time.DateOnly) != value || date.Year() < 1 {
		return DeliveryDate{}, ErrInvalidDate
	}
	return DeliveryDate{date: value}, nil
}

func (d DeliveryDate) String() string {
	return d.date
}

// BeforeLocalDay compares calendar dates in the location supplied by the caller.
// It never changes the delivery date by converting it to a UTC instant.
func (d DeliveryDate) BeforeLocalDay(instant time.Time) bool {
	return d.String() < instant.Format(time.DateOnly)
}

// DeliveryWindow contains local hours on a single delivery date.
type DeliveryWindow struct {
	start string
	end   string
}

func NewDeliveryWindow(start, end string) (DeliveryWindow, error) {
	if !validClock(start) || !validClock(end) || end <= start {
		return DeliveryWindow{}, ErrInvalidWindow
	}
	return DeliveryWindow{start: start, end: end}, nil
}

func (w DeliveryWindow) Start() string { return w.start }
func (w DeliveryWindow) End() string   { return w.end }

func validClock(value string) bool {
	if len(value) != 5 {
		return false
	}
	clock, err := time.Parse("15:04", value)
	return err == nil && clock.Format("15:04") == value
}
