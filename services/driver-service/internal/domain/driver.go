package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Límites de validación, exportados para que las pruebas y los transportes no los dupliquen.
const (
	// MaxFullNameRunes es el largo máximo del nombre completo, en runas.
	MaxFullNameRunes = 120
	// MinPhoneDigits es la cantidad mínima de dígitos de un teléfono, sin contar el +.
	MinPhoneDigits = 7
	// MaxPhoneDigits es la cantidad máxima de dígitos de un teléfono, sin contar el +.
	MaxPhoneDigits = 15
)

// Driver es un repartidor del sistema.
type Driver struct {
	ID string
	// UserID referencia al usuario del Auth Service con rol DRIVER.
	UserID   string
	FullName string
	Phone    string
	Active   bool
	Status   Status
	// RouteID es la ruta para la que el repartidor está reservado; vacío si no tiene ninguna.
	RouteID   string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewDriverInput contiene los datos necesarios para registrar un repartidor.
type NewDriverInput struct {
	UserID   string
	FullName string
	Phone    string
}

// NormalizedNewDriver es la entrada validada y normalizada.
type NormalizedNewDriver struct {
	UserID   string
	FullName string
	Phone    string
}

// Validate valida y normaliza los datos de un repartidor nuevo.
func (in NewDriverInput) Validate() (NormalizedNewDriver, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return NormalizedNewDriver{}, ErrInvalidUserID
	}
	name := strings.TrimSpace(in.FullName)
	if name == "" || utf8.RuneCountInString(name) > MaxFullNameRunes {
		return NormalizedNewDriver{}, ErrInvalidFullName
	}
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return NormalizedNewDriver{}, err
	}
	return NormalizedNewDriver{UserID: userID, FullName: name, Phone: phone}, nil
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
	n, err := in.Validate()
	if err != nil {
		return nil, err
	}
	return &Driver{
		ID:        id,
		UserID:    n.UserID,
		FullName:  n.FullName,
		Phone:     n.Phone,
		Active:    true,
		Status:    StatusAvailable,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
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
