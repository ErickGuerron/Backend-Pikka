package domain

import (
	"strings"
	"unicode/utf8"
)

// UserUpdate son los campos editables por un ADMIN. Nil significa "no cambiar".
type UserUpdate struct {
	FullName *string
	Role     *Role
}

// ApplyUpdate valida y aplica una edición sobre cur. activeAdmins es el número
// de ADMIN activos en el sistema (incluido cur si lo es).
func ApplyUpdate(cur User, activeAdmins int, in UserUpdate) (User, error) {
	if in.FullName == nil && in.Role == nil {
		return User{}, ErrNothingToUpdate
	}
	next := cur
	if in.FullName != nil {
		name := strings.TrimSpace(*in.FullName)
		if name == "" || utf8.RuneCountInString(name) > maxFullNameRunes {
			return User{}, ErrInvalidFullName
		}
		next.FullName = name
	}
	if in.Role != nil {
		if err := checkRoleChange(cur, activeAdmins, *in.Role); err != nil {
			return User{}, err
		}
		next.Role = *in.Role
	}
	return next, nil
}

// ApplyDeactivation hace borrado lógico (active=false). Es idempotente: un
// usuario ya inactivo se devuelve tal cual.
func ApplyDeactivation(cur User, activeAdmins int, actorID string) (User, error) {
	if cur.ID == actorID {
		return User{}, ErrSelfDeactivation
	}
	if !cur.Active {
		return cur, nil
	}
	if cur.Role == RoleAdmin && activeAdmins <= 1 {
		return User{}, ErrLastActiveAdmin
	}
	next := cur
	next.Active = false
	return next, nil
}

// checkRoleChange aplica las reglas de rol: CUSTOMER es inmutable y nunca se
// asigna por esta vía; el último ADMIN activo no puede dejar de serlo.
func checkRoleChange(cur User, activeAdmins int, role Role) error {
	if !role.Valid() {
		return ErrInvalidRole
	}
	if role == RoleCustomer {
		return ErrCustomerRoleNotAssignable
	}
	if cur.Role == RoleCustomer {
		return ErrCustomerRoleLocked
	}
	if cur.Role == RoleAdmin && cur.Active && role != RoleAdmin && activeAdmins <= 1 {
		return ErrLastActiveAdmin
	}
	return nil
}
