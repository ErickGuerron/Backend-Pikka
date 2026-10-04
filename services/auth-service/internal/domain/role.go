package domain

// Role es el rol de un usuario dentro del sistema.
type Role string

const (
	RoleAdmin    Role = "ADMIN"
	RoleOperator Role = "OPERATOR"
	RoleDriver   Role = "DRIVER"
)

// ParseRole convierte un texto en un Role válido.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if !r.Valid() {
		return "", ErrInvalidRole
	}
	return r, nil
}

// Valid indica si el rol pertenece al conjunto de roles soportados.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleOperator, RoleDriver:
		return true
	default:
		return false
	}
}
