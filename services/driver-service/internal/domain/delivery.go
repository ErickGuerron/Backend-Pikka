package domain

import (
	"errors"
	"time"
)

// Outcome es el resultado de una parada.
type Outcome string

const (
	OutcomeDelivered    Outcome = "DELIVERED"
	OutcomeNotDelivered Outcome = "NOT_DELIVERED"
)

// NotDeliveredReason es el motivo cerrado de una entrega fallida (ADR 0007, sección 4).
type NotDeliveredReason string

const (
	ReasonClientRefused  NotDeliveredReason = "CLIENT_REFUSED"
	ReasonClientAbsent   NotDeliveredReason = "CLIENT_ABSENT"
	ReasonOutOfWindow    NotDeliveredReason = "OUT_OF_WINDOW"
	ReasonProductDamage  NotDeliveredReason = "PRODUCT_DAMAGED"
	ReasonWrongAddress   NotDeliveredReason = "WRONG_ADDRESS"
	ReasonDuplicateOrder NotDeliveredReason = "DUPLICATE_ORDER"
)

// Imputable indica de quién es la responsabilidad del motivo. El recargo solo aplica a
// motivos del cliente; el sistema decide la penalización, no el repartidor.
type Imputable string

const (
	ImputableClient   Imputable = "CLIENT"
	ImputableBusiness Imputable = "BUSINESS"
)

var reasonImputable = map[NotDeliveredReason]Imputable{
	ReasonClientRefused:  ImputableClient,
	ReasonClientAbsent:   ImputableClient,
	ReasonOutOfWindow:    ImputableClient,
	ReasonProductDamage:  ImputableBusiness,
	ReasonWrongAddress:   ImputableBusiness,
	ReasonDuplicateOrder: ImputableBusiness,
}

var (
	ErrInvalidOutcome   = errors.New("outcome must be DELIVERED or NOT_DELIVERED")
	ErrReasonRequired   = errors.New("NOT_DELIVERED requires a reason from the closed list")
	ErrReasonNotAllowed = errors.New("DELIVERED must not carry a reason")
	ErrInvalidStopID    = errors.New("stop id is required")
	ErrInvalidWindow    = errors.New("window start must be before window end")
	ErrInvalidRadius    = errors.New("radius must be positive")
)

// ImputableOf devuelve de quién es el motivo, o false si el motivo no pertenece a la lista.
func ImputableOf(r NotDeliveredReason) (Imputable, bool) {
	i, ok := reasonImputable[r]
	return i, ok
}

// DeliveryRecord es el registro que el repartidor hace de una parada.
type DeliveryRecord struct {
	StopID   string
	Outcome  Outcome
	Reason   NotDeliveredReason
	At       time.Time
	Location GeoPoint
	// PhotoURL es opcional y vacío si no se adjuntó foto.
	PhotoURL string
}

// NewDeliveryRecord valida un registro de entrega. Un DELIVERED no lleva motivo; un
// NOT_DELIVERED exige un motivo de la lista cerrada. La ubicación GPS es obligatoria.
func NewDeliveryRecord(stopID string, outcome Outcome, reason NotDeliveredReason, at time.Time, loc GeoPoint, photoURL string) (DeliveryRecord, error) {
	if stopID == "" {
		return DeliveryRecord{}, ErrInvalidStopID
	}
	if err := loc.Validate(); err != nil {
		return DeliveryRecord{}, err
	}
	switch outcome {
	case OutcomeDelivered:
		if reason != "" {
			return DeliveryRecord{}, ErrReasonNotAllowed
		}
	case OutcomeNotDelivered:
		if _, ok := ImputableOf(reason); !ok {
			return DeliveryRecord{}, ErrReasonRequired
		}
	default:
		return DeliveryRecord{}, ErrInvalidOutcome
	}
	return DeliveryRecord{StopID: stopID, Outcome: outcome, Reason: reason, At: at, Location: loc, PhotoURL: photoURL}, nil
}

// DefaultArrivalRadiusMeters es el radio de llegada por defecto. Es un valor propuesto, no
// definido en los documentos: se cambia con DRIVER_ARRIVAL_RADIUS_METERS.
const DefaultArrivalRadiusMeters float64 = 50

// ArrivalWindow es la franja horaria de la parada, en la misma zona horaria que At.
type ArrivalWindow struct {
	Start time.Time
	End   time.Time
}

// Stop es la dirección a la que llega el repartidor.
type Stop struct {
	Location GeoPoint
	Window   ArrivalWindow
}

// ArrivalCheck es el resultado de verificar la llegada antes de cualquier penalización.
type ArrivalCheck struct {
	WithinRadius bool
	WithinWindow bool
	DistanceM    float64
}

// Valid indica si la llegada cuenta: dentro del radio y dentro de la ventana.
func (c ArrivalCheck) Valid() bool {
	return c.WithinRadius && c.WithinWindow
}

// VerifyArrival comprueba si el registro cae dentro del radio de la parada y de su ventana.
// El radio llega por parámetro porque su valor aún no está definido en los documentos.
func VerifyArrival(stop Stop, rec DeliveryRecord, radiusMeters float64) (ArrivalCheck, error) {
	if radiusMeters <= 0 {
		return ArrivalCheck{}, ErrInvalidRadius
	}
	if !stop.Window.Start.Before(stop.Window.End) {
		return ArrivalCheck{}, ErrInvalidWindow
	}
	dist := DistanceMeters(stop.Location, rec.Location)
	return ArrivalCheck{
		WithinRadius: dist <= radiusMeters,
		WithinWindow: !rec.At.Before(stop.Window.Start) && rec.At.Before(stop.Window.End),
		DistanceM:    dist,
	}, nil
}
