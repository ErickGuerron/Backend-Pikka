package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Límites de validación, exportados para que las pruebas y los transportes no los dupliquen.
const (
	// MinPhoneDigits es la cantidad mínima de dígitos de un teléfono, sin contar el +.
	MinPhoneDigits = 7
	// MaxPhoneDigits es la cantidad máxima de dígitos de un teléfono, sin contar el +.
	MaxPhoneDigits = 15
	// MaxNameRunes es el largo máximo del nombre y del apellido, en runas; juntos caben en full_name (120).
	MaxNameRunes = 50
	// MaxCityRunes es el largo máximo de la ciudad, en runas.
	MaxCityRunes = 80
	// MinDriverAge es la edad mínima, en años cumplidos, para registrarse como repartidor.
	MinDriverAge = 18
	// MinIdentifierLen y MaxIdentifierLen acotan el documento de identidad (letras y dígitos).
	MinIdentifierLen = 5
	MaxIdentifierLen = 20
	// MinPlateLen y MaxPlateLen acotan la placa del vehículo (letras y dígitos, sin guiones).
	MinPlateLen = 5
	MaxPlateLen = 8
)

// Driver es un repartidor del sistema.
type Driver struct {
	ID string
	// UserID referencia al usuario del Auth Service con rol DRIVER.
	UserID string
	// FullName es "FirstName LastName"; se conserva porque otros servicios ya lo consumen.
	FullName  string
	FirstName string
	LastName  string
	// BirthDate es la fecha de nacimiento, a medianoche UTC.
	BirthDate time.Time
	// Phone es el número de contacto.
	Phone       string
	VehicleType VehicleType
	// Identifier es el documento de identidad, en mayúsculas.
	Identifier string
	// Plate es la placa del vehículo, en mayúsculas y sin guiones.
	Plate  string
	City   string
	Active bool
	Status Status
	// RouteID es la ruta para la que el repartidor está reservado; vacío si no tiene ninguna.
	RouteID   string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewDriverInput contiene los datos necesarios para registrar un repartidor.
type NewDriverInput struct {
	UserID      string
	FirstName   string
	LastName    string
	BirthDate   time.Time
	Phone       string
	VehicleType string
	Identifier  string
	Plate       string
	City        string
}

// NormalizedNewDriver es la entrada validada y normalizada.
type NormalizedNewDriver struct {
	UserID      string
	FullName    string
	FirstName   string
	LastName    string
	BirthDate   time.Time
	Phone       string
	VehicleType VehicleType
	Identifier  string
	Plate       string
	City        string
}

// Validate valida y normaliza los datos de un repartidor nuevo. now sirve para comprobar la edad.
func (in NewDriverInput) Validate(now time.Time) (NormalizedNewDriver, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return NormalizedNewDriver{}, ErrInvalidUserID
	}
	first := strings.TrimSpace(in.FirstName)
	if first == "" || utf8.RuneCountInString(first) > MaxNameRunes {
		return NormalizedNewDriver{}, ErrInvalidFirstName
	}
	last := strings.TrimSpace(in.LastName)
	if last == "" || utf8.RuneCountInString(last) > MaxNameRunes {
		return NormalizedNewDriver{}, ErrInvalidLastName
	}
	birth, err := normalizeBirthDate(in.BirthDate, now)
	if err != nil {
		return NormalizedNewDriver{}, err
	}
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return NormalizedNewDriver{}, err
	}
	vehicle, err := ParseVehicleType(in.VehicleType)
	if err != nil {
		return NormalizedNewDriver{}, err
	}
	identifier, ok := alphanumeric(in.Identifier, MinIdentifierLen, MaxIdentifierLen)
	if !ok {
		return NormalizedNewDriver{}, ErrInvalidIdentifier
	}
	plate, ok := alphanumeric(in.Plate, MinPlateLen, MaxPlateLen)
	if !ok {
		return NormalizedNewDriver{}, ErrInvalidPlate
	}
	city := strings.TrimSpace(in.City)
	if city == "" || utf8.RuneCountInString(city) > MaxCityRunes {
		return NormalizedNewDriver{}, ErrInvalidCity
	}
	return NormalizedNewDriver{
		UserID: userID, FullName: first + " " + last, FirstName: first, LastName: last,
		BirthDate: birth, Phone: phone, VehicleType: vehicle, Identifier: identifier, Plate: plate, City: city,
	}, nil
}

// normalizeBirthDate deja solo la fecha (UTC) y exige que sea pasada y que el repartidor tenga al menos MinDriverAge años.
func normalizeBirthDate(raw, now time.Time) (time.Time, error) {
	if raw.IsZero() {
		return time.Time{}, ErrInvalidBirthDate
	}
	y, m, d := raw.Date()
	birth := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	ny, nm, nd := now.UTC().Date()
	today := time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)
	if birth.After(today.AddDate(-MinDriverAge, 0, 0)) {
		return time.Time{}, ErrInvalidBirthDate
	}
	return birth, nil
}

// alphanumeric quita espacios y guiones, pasa a mayúsculas y comprueba largo y que sean solo letras ASCII y dígitos.
func alphanumeric(raw string, minLen, maxLen int) (string, bool) {
	v := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw)))
	if len(v) < minLen || len(v) > maxLen {
		return "", false
	}
	for _, r := range v {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return "", false
		}
	}
	return v, true
}

// NormalizePhone quita espacios y guiones y hace una validación simplificada: un +
// opcional al inicio y entre 7 y 15 dígitos. No valida el código de país ni el plan
// de numeración, por lo que no garantiza un número E.164 real.
func NormalizePhone(raw string) (string, error) {
	phone := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	digits := strings.TrimPrefix(phone, "+")
	if len(digits) < MinPhoneDigits || len(digits) > MaxPhoneDigits {
		return "", ErrInvalidPhone
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", ErrInvalidPhone
		}
	}
	return phone, nil
}

// NewDriver crea un repartidor activo y disponible a partir de datos válidos:
// se registra y se le asigna trabajo poco después, por lo que no nace fuera de línea.
// El instante now se inyecta para que el dominio no dependa del reloj del sistema.
func NewDriver(id string, in NewDriverInput, now time.Time) (*Driver, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidDriverID
	}
	n, err := in.Validate(now)
	if err != nil {
		return nil, err
	}
	return &Driver{
		ID:          id,
		UserID:      n.UserID,
		FullName:    n.FullName,
		FirstName:   n.FirstName,
		LastName:    n.LastName,
		BirthDate:   n.BirthDate,
		Phone:       n.Phone,
		VehicleType: n.VehicleType,
		Identifier:  n.Identifier,
		Plate:       n.Plate,
		City:        n.City,
		Active:      true,
		Status:      StatusAvailable,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Reserve asigna el repartidor a una ruta. Un repartidor ya reservado solo admite la
// misma ruta: repetirla es un reintento idempotente que devuelve nil sin cambiar el
// estado ni la versión, incluso si el repartidor fue desactivado después, porque la
// reserva original ya tuvo éxito. Otra ruta se rechaza con ErrDriverAlreadyReserved
// (también si está inactivo) porque no puede atender dos pedidos a la vez. Una reserva
// nueva exige que el repartidor esté activo.
//
// Límite conocido: la idempotencia solo se cumple mientras el repartidor conserva la
// ruta. Una vez liberada o completada, un Reserve duplicado y tardío con esa ruta
// antigua volvería a reservar al repartidor. Por eso la capa de aplicación debe
// deduplicar por clave de idempotencia (BASE_TECNICA 16.4) y no llamar a Reserve
// para una ruta ya finalizada.
func (d *Driver) Reserve(routeID string, now time.Time) error {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return ErrInvalidRouteID
	}
	if d.Status == StatusAssigned || d.Status == StatusOnRoute {
		if d.RouteID == routeID {
			return nil
		}
		return ErrDriverAlreadyReserved
	}
	if !d.Active {
		return ErrDriverInactive
	}
	if d.Status == StatusOffline {
		return ErrDriverOffline
	}
	if d.Status != StatusAvailable {
		return ErrInvalidTransition
	}
	d.apply(StatusAssigned, now)
	d.RouteID = routeID
	return nil
}

// Release libera al repartidor de la ruta reservada routeID, que no se inició, y lo deja
// AVAILABLE, o OFFLINE si fue desactivado mientras tenía la reserva. Orden de errores:
// ErrInvalidRouteID, ErrInvalidTransition (no está ASSIGNED) y ErrRouteMismatch.
func (d *Driver) Release(routeID string, now time.Time) error {
	if err := d.checkReservation(routeID, StatusAssigned); err != nil {
		return err
	}
	return d.finishReservation(now)
}

// StartRoute marca el inicio de la ruta routeID de un repartidor asignado. Un repartidor
// inactivo no puede iniciarla; si ya la tenía reservada, aún puede liberarla o completarla.
// Orden de errores: ErrInvalidRouteID, ErrDriverInactive, ErrInvalidTransition (no está
// ASSIGNED) y ErrRouteMismatch.
func (d *Driver) StartRoute(routeID string, now time.Time) error {
	if strings.TrimSpace(routeID) == "" {
		return ErrInvalidRouteID
	}
	if !d.Active {
		return ErrDriverInactive
	}
	if err := d.checkReservation(routeID, StatusAssigned); err != nil {
		return err
	}
	return d.transition(StatusOnRoute, now)
}

// CompleteRoute marca el fin de la ruta routeID y deja al repartidor AVAILABLE, o OFFLINE
// si fue desactivado mientras estaba en ruta. Orden de errores: ErrInvalidRouteID,
// ErrInvalidTransition (no está ON_ROUTE) y ErrRouteMismatch.
func (d *Driver) CompleteRoute(routeID string, now time.Time) error {
	if err := d.checkReservation(routeID, StatusOnRoute); err != nil {
		return err
	}
	return d.finishReservation(now)
}

// GoOffline retira al repartidor de la operación. No es posible estando en ruta.
//
// Estando ASSIGNED, pasar a OFFLINE descarta la reserva (RouteID queda vacío). No está
// prohibido, pero la capa de aplicación debe avisar al Routing Service para que el
// pedido reciba otro repartidor.
func (d *Driver) GoOffline(now time.Time) error {
	return d.transition(StatusOffline, now)
}

// GoOnline devuelve a un repartidor fuera de línea a la operación.
func (d *Driver) GoOnline(now time.Time) error {
	// La comprobación de Active va antes que la del estado de origen: un repartidor
	// inactivo nunca debe quedar AVAILABLE sea cual sea su estado, así que
	// ErrDriverInactive tiene prioridad sobre ErrInvalidTransition.
	if !d.Active {
		return ErrDriverInactive
	}
	if d.Status != StatusOffline {
		return ErrInvalidTransition
	}
	return d.transition(StatusAvailable, now)
}

// Deactivate retira al repartidor del sistema. Es idempotente: sobre un repartidor ya
// inactivo no cambia nada ni la versión.
//
// Un repartidor AVAILABLE pasa a OFFLINE, porque un inactivo nunca queda AVAILABLE. Si
// tiene una reserva o una ruta en curso, el estado no cambia: la operación termina con
// Release o CompleteRoute, que lo dejan OFFLINE por finishReservation.
func (d *Driver) Deactivate(now time.Time) {
	if !d.Active {
		return
	}
	d.Active = false
	if d.Status == StatusAvailable {
		d.apply(StatusOffline, now)
		return
	}
	d.Version++
	d.UpdatedAt = now
}

// Activate vuelve a habilitar al repartidor. Es idempotente: sobre uno ya activo no cambia
// nada ni la versión. El estado no cambia: un repartidor reactivado sigue OFFLINE hasta
// que él mismo vuelva a conectarse con GoOnline.
func (d *Driver) Activate(now time.Time) {
	if d.Active {
		return
	}
	d.Active = true
	d.Version++
	d.UpdatedAt = now
}

// checkReservation valida que routeID no esté en blanco, que el repartidor esté en el
// estado from y que routeID sea la ruta reservada, en ese orden. No modifica al repartidor.
func (d *Driver) checkReservation(routeID string, from Status) error {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return ErrInvalidRouteID
	}
	if d.Status != from {
		return ErrInvalidTransition
	}
	if d.RouteID != routeID {
		return ErrRouteMismatch
	}
	return nil
}

// finishReservation cierra una reserva ya validada (Release o CompleteRoute).
// Invariante: un repartidor inactivo nunca queda AVAILABLE, así que si fue
// desactivado con trabajo pendiente la operación igual se completa, pero termina
// OFFLINE. Por eso aplica el cambio directamente: ON_ROUTE -> OFFLINE sigue
// prohibido en la tabla pública y solo se permite por este camino.
func (d *Driver) finishReservation(now time.Time) error {
	if !d.Active {
		d.apply(StatusOffline, now)
		return nil
	}
	return d.transition(StatusAvailable, now)
}

func (d *Driver) transition(next Status, now time.Time) error {
	if !d.Status.CanTransitionTo(next) {
		return ErrInvalidTransition
	}
	d.apply(next, now)
	return nil
}

func (d *Driver) apply(next Status, now time.Time) {
	d.Status = next
	if next != StatusAssigned && next != StatusOnRoute {
		d.RouteID = ""
	}
	d.Version++
	d.UpdatedAt = now
}
