package domain

import "time"

// NewStop construye la parada a partir de lo que entrega Order Service: la ubicacion y la ventana
// en horas locales (HH:mm) de la fecha de entrega y zona horaria operativa.
func NewStop(location GeoPoint, deliveryDate, start, end string, timezone *time.Location) (Stop, error) {
	day, err := time.ParseInLocation(time.DateOnly, deliveryDate, timezone)
	if err != nil {
		return Stop{}, ErrInvalidWindow
	}
	from, err := clockOn(day, start)
	if err != nil {
		return Stop{}, err
	}
	to, err := clockOn(day, end)
	if err != nil {
		return Stop{}, err
	}
	if !from.Before(to) {
		return Stop{}, ErrInvalidWindow
	}
	return Stop{Location: location, Window: ArrivalWindow{Start: from, End: to}}, nil
}

// clockOn coloca una hora HH:mm sobre el dia dado, conservando su zona horaria.
func clockOn(day time.Time, clock string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02 15:04", day.Format(time.DateOnly)+" "+clock, day.Location())
	if err != nil || t.Format("15:04") != clock {
		return time.Time{}, ErrInvalidWindow
	}
	return t, nil
}
