package domain

// Status es el estado operativo de un repartidor.
type Status string

const (
	StatusAvailable Status = "AVAILABLE"
	StatusAssigned  Status = "ASSIGNED"
	StatusOnRoute   Status = "ON_ROUTE"
	StatusOffline   Status = "OFFLINE"
)

// allowedTransitions define los cambios de estado permitidos. Un repartidor en
// ruta no puede pasar a OFFLINE y los cambios al mismo estado se rechazan.
var allowedTransitions = map[Status][]Status{
	StatusAvailable: {StatusAssigned, StatusOffline},
	StatusAssigned:  {StatusOnRoute, StatusAvailable, StatusOffline},
	StatusOnRoute:   {StatusAvailable},
	StatusOffline:   {StatusAvailable},
}

// ParseStatus convierte un texto en un estado válido, sin normalizar mayúsculas.
func ParseStatus(raw string) (Status, error) {
	s := Status(raw)
	if !s.Valid() {
		return "", ErrInvalidStatus
	}
	return s, nil
}

// Valid indica si el estado es uno de los conocidos.
func (s Status) Valid() bool {
	_, ok := allowedTransitions[s]
	return ok
}

// CanTransitionTo indica si el paso de s a next está permitido.
func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}
