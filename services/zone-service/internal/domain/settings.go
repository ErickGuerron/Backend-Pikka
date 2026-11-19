package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // zonas horarias IANA disponibles aunque el host (Windows, contenedores) no las tenga
	"unicode/utf8"
)

// Límites de la configuración. Reflejan una operación real en Ecuador: una furgoneta o moto no carga
// más de 50 pedidos ni más de una tonelada, y el pico de una fecha como San Valentín llega a miles de pedidos.
const (
	maxDescriptionLength = 500
	minDepotNameLength   = 3
	maxDepotNameLength   = 120
	defaultTimezone      = "America/Guayaquil"
	timeLayout           = "15:04"

	maxOrdersPerRouteLimit = 50
	maxRouteCapacityKg     = 1000
	maxDailyOrderLimit     = 10000

	// Ecuador continental y Galápagos.
	ecuadorMinLatitude  = -5.0
	ecuadorMaxLatitude  = 2.0
	ecuadorMinLongitude = -92.0
	ecuadorMaxLongitude = -75.0
)

// depotNamePattern: solo letras (con acentos), espacios y . - '. La bodega se nombra con palabras.
var depotNamePattern = regexp.MustCompile(`^[\p{L} .'\-]+$`)

// Prioridades de despacho de una zona.
const (
	PriorityNormal = "NORMAL"
	PriorityHigh   = "HIGH"
	PriorityLow    = "LOW"
)

// Días de servicio válidos, en el orden de la semana.
var weekdays = []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"}

// Settings es la configuración operativa de una zona: depósito de salida, capacidad de ruta,
// horario de servicio, cortes de pedidos y prioridad. Solo el administrador la define.
type Settings struct {
	Description        string
	DepotName          string
	DepotLatitude      float64
	DepotLongitude     float64
	MaxOrdersPerRoute  int
	RouteCapacityKg    float64
	Timezone           string
	ServiceDays        []string
	DeliveryWindowFrom string // HH:MM local
	DeliveryWindowTo   string // HH:MM local
	CutoffTime         string // HH:MM local: hasta aquí entran pedidos para la fecha
	DispatchTime       string // HH:MM local: hora en que salen las rutas
	Priority           string
	DailyOrderLimit    int
}

// ValidateSettings normaliza la configuración (espacios, mayúsculas, orden de días) y la valida.
// Devuelve la configuración normalizada. Todo error envuelve ErrInvalidSettings con el detalle.
func ValidateSettings(s Settings) (Settings, error) {
	out := s
	out.Description = strings.TrimSpace(s.Description)
	if len(out.Description) > maxDescriptionLength {
		return Settings{}, invalidSettings("description must be at most 500 characters")
	}

	out.DepotName = strings.TrimSpace(s.DepotName)
	depotLength := utf8.RuneCountInString(out.DepotName)
	if depotLength < minDepotNameLength || depotLength > maxDepotNameLength || !depotNamePattern.MatchString(out.DepotName) {
		return Settings{}, invalidSettings("depot name is required, 3 to 120 letters; only letters, spaces, . - ' are allowed")
	}
	if _, err := NewCoordinate(s.DepotLatitude, s.DepotLongitude); err != nil {
		return Settings{}, invalidSettings("depot coordinates are out of range")
	}
	if s.DepotLatitude < ecuadorMinLatitude || s.DepotLatitude > ecuadorMaxLatitude ||
		s.DepotLongitude < ecuadorMinLongitude || s.DepotLongitude > ecuadorMaxLongitude {
		return Settings{}, invalidSettings("depot must be inside Ecuador (lat -5 to 2, lng -92 to -75)")
	}

	if s.MaxOrdersPerRoute < 1 || s.MaxOrdersPerRoute > maxOrdersPerRouteLimit {
		return Settings{}, invalidSettings("max orders per route must be between 1 and 50")
	}
	if math.IsNaN(s.RouteCapacityKg) || math.IsInf(s.RouteCapacityKg, 0) || s.RouteCapacityKg <= 0 || s.RouteCapacityKg > maxRouteCapacityKg {
		return Settings{}, invalidSettings("route capacity must be between 1 and 1000 kilograms")
	}

	out.Timezone = strings.TrimSpace(s.Timezone)
	if out.Timezone == "" {
		out.Timezone = defaultTimezone
	}
	if _, err := time.LoadLocation(out.Timezone); err != nil {
		return Settings{}, invalidSettings("timezone must be a valid IANA name, e.g. America/Guayaquil")
	}

	days, err := normalizeServiceDays(s.ServiceDays)
	if err != nil {
		return Settings{}, err
	}
	out.ServiceDays = days

	from, err := parseClock(s.DeliveryWindowFrom, "delivery window start")
	if err != nil {
		return Settings{}, err
	}
	to, err := parseClock(s.DeliveryWindowTo, "delivery window end")
	if err != nil {
		return Settings{}, err
	}
	if !from.Before(to) {
		return Settings{}, invalidSettings("delivery window start must be before its end")
	}
	out.DeliveryWindowFrom = from.Format(timeLayout)
	out.DeliveryWindowTo = to.Format(timeLayout)

	cutoff, err := parseClock(s.CutoffTime, "cutoff time")
	if err != nil {
		return Settings{}, err
	}
	dispatch, err := parseClock(s.DispatchTime, "dispatch time")
	if err != nil {
		return Settings{}, err
	}
	out.CutoffTime = cutoff.Format(timeLayout)
	out.DispatchTime = dispatch.Format(timeLayout)

	out.Priority = strings.ToUpper(strings.TrimSpace(s.Priority))
	if out.Priority == "" {
		out.Priority = PriorityNormal
	}
	if out.Priority != PriorityNormal && out.Priority != PriorityHigh && out.Priority != PriorityLow {
		return Settings{}, invalidSettings("priority must be NORMAL, HIGH or LOW")
	}

	if s.DailyOrderLimit < 1 || s.DailyOrderLimit > maxDailyOrderLimit {
		return Settings{}, invalidSettings("daily order limit must be between 1 and 10000")
	}
	return out, nil
}

// normalizeServiceDays exige al menos un día, solo días válidos y sin repetidos; los devuelve en
// orden de semana.
func normalizeServiceDays(days []string) ([]string, error) {
	if len(days) == 0 {
		return nil, invalidSettings("at least one service day is required")
	}
	seen := make(map[string]bool, len(days))
	for _, d := range days {
		key := strings.ToUpper(strings.TrimSpace(d))
		if seen[key] {
			return nil, invalidSettings("service days must not repeat")
		}
		seen[key] = true
	}
	out := make([]string, 0, len(seen))
	for _, wd := range weekdays {
		if seen[wd] {
			out = append(out, wd)
			delete(seen, wd)
		}
	}
	if len(seen) > 0 {
		return nil, invalidSettings("service days must be MON, TUE, WED, THU, FRI, SAT or SUN")
	}
	return out, nil
}

func parseClock(value, field string) (time.Time, error) {
	t, err := time.Parse(timeLayout, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, invalidSettings(fmt.Sprintf("%s must be a time in HH:MM format", field))
	}
	return t, nil
}

func invalidSettings(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSettings, detail)
}
