package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

func activeUser(id string, role domain.Role) domain.User {
	return domain.User{ID: id, Email: id + "@example.com", FullName: "Nombre", Role: role, Active: true, Version: 1}
}

func strPtr(s string) *string { return &s }

func rolePtr(r domain.Role) *domain.Role { return &r }

func TestApplyUpdate_ChangesFullNameAndTrims(t *testing.T) {
	cur := activeUser("u-1", domain.RoleDriver)

	got, err := domain.ApplyUpdate(cur, 1, domain.UserUpdate{FullName: strPtr("  Nuevo Nombre  ")})
	require.NoError(t, err)
	assert.Equal(t, "Nuevo Nombre", got.FullName)
	assert.Equal(t, domain.RoleDriver, got.Role, "el rol no cambia si no se pide")
}

func TestApplyUpdate_RejectsEmptyUpdate(t *testing.T) {
	_, err := domain.ApplyUpdate(activeUser("u-1", domain.RoleDriver), 1, domain.UserUpdate{})
	assert.ErrorIs(t, err, domain.ErrNothingToUpdate)
}

func TestApplyUpdate_RejectsInvalidFullName(t *testing.T) {
	cur := activeUser("u-1", domain.RoleDriver)

	_, err := domain.ApplyUpdate(cur, 1, domain.UserUpdate{FullName: strPtr("   ")})
	assert.ErrorIs(t, err, domain.ErrInvalidFullName)

	_, err = domain.ApplyUpdate(cur, 1, domain.UserUpdate{FullName: strPtr(strings.Repeat("a", 121))})
	assert.ErrorIs(t, err, domain.ErrInvalidFullName)
}

func TestApplyUpdate_RejectsUnknownRole(t *testing.T) {
	_, err := domain.ApplyUpdate(activeUser("u-1", domain.RoleDriver), 1, domain.UserUpdate{Role: rolePtr("ROOT")})
	assert.ErrorIs(t, err, domain.ErrInvalidRole)
}

func TestApplyUpdate_ChangesRoleBetweenStaffRoles(t *testing.T) {
	got, err := domain.ApplyUpdate(activeUser("u-1", domain.RoleDriver), 1, domain.UserUpdate{Role: rolePtr(domain.RoleOperator)})
	require.NoError(t, err)
	assert.Equal(t, domain.RoleOperator, got.Role)
}

func TestApplyUpdate_LastActiveAdminCannotChangeRole(t *testing.T) {
	_, err := domain.ApplyUpdate(activeUser("u-admin", domain.RoleAdmin), 1, domain.UserUpdate{Role: rolePtr(domain.RoleOperator)})
	assert.ErrorIs(t, err, domain.ErrLastActiveAdmin)
}

func TestApplyUpdate_AdminCanChangeRoleWhenAnotherActiveAdminExists(t *testing.T) {
	got, err := domain.ApplyUpdate(activeUser("u-admin", domain.RoleAdmin), 2, domain.UserUpdate{Role: rolePtr(domain.RoleOperator)})
	require.NoError(t, err)
	assert.Equal(t, domain.RoleOperator, got.Role)
}

func TestApplyUpdate_InactiveAdminRoleChangeDoesNotRemoveAnActiveAdmin(t *testing.T) {
	cur := activeUser("u-old", domain.RoleAdmin)
	cur.Active = false

	got, err := domain.ApplyUpdate(cur, 1, domain.UserUpdate{Role: rolePtr(domain.RoleOperator)})
	require.NoError(t, err)
	assert.Equal(t, domain.RoleOperator, got.Role)
}

func TestApplyUpdate_CustomerRoleIsLocked(t *testing.T) {
	_, err := domain.ApplyUpdate(activeUser("u-c", domain.RoleCustomer), 1, domain.UserUpdate{Role: rolePtr(domain.RoleDriver)})
	assert.ErrorIs(t, err, domain.ErrCustomerRoleLocked)
}

func TestApplyUpdate_CustomerRoleCannotBeAssigned(t *testing.T) {
	_, err := domain.ApplyUpdate(activeUser("u-d", domain.RoleDriver), 1, domain.UserUpdate{Role: rolePtr(domain.RoleCustomer)})
	assert.ErrorIs(t, err, domain.ErrCustomerRoleNotAssignable)

	_, err = domain.ApplyUpdate(activeUser("u-c", domain.RoleCustomer), 1, domain.UserUpdate{Role: rolePtr(domain.RoleCustomer)})
	assert.ErrorIs(t, err, domain.ErrCustomerRoleNotAssignable)
}

func TestApplyUpdate_CustomerNameCanBeEdited(t *testing.T) {
	got, err := domain.ApplyUpdate(activeUser("u-c", domain.RoleCustomer), 1, domain.UserUpdate{FullName: strPtr("Cliente Nuevo")})
	require.NoError(t, err)
	assert.Equal(t, "Cliente Nuevo", got.FullName)
	assert.Equal(t, domain.RoleCustomer, got.Role)
}

func TestApplyDeactivation_SetsActiveFalse(t *testing.T) {
	got, err := domain.ApplyDeactivation(activeUser("u-d", domain.RoleDriver), 1, "u-admin")
	require.NoError(t, err)
	assert.False(t, got.Active)
	assert.Equal(t, domain.RoleDriver, got.Role)
}

func TestApplyDeactivation_CustomerCanBeDeactivated(t *testing.T) {
	got, err := domain.ApplyDeactivation(activeUser("u-c", domain.RoleCustomer), 1, "u-admin")
	require.NoError(t, err)
	assert.False(t, got.Active)
}

func TestApplyDeactivation_SelfIsForbidden(t *testing.T) {
	_, err := domain.ApplyDeactivation(activeUser("u-admin", domain.RoleAdmin), 2, "u-admin")
	assert.ErrorIs(t, err, domain.ErrSelfDeactivation)
}

func TestApplyDeactivation_LastActiveAdminIsForbidden(t *testing.T) {
	_, err := domain.ApplyDeactivation(activeUser("u-other", domain.RoleAdmin), 1, "u-admin")
	assert.ErrorIs(t, err, domain.ErrLastActiveAdmin)
}

func TestApplyDeactivation_AdminAllowedWhenAnotherActiveAdminExists(t *testing.T) {
	got, err := domain.ApplyDeactivation(activeUser("u-other", domain.RoleAdmin), 2, "u-admin")
	require.NoError(t, err)
	assert.False(t, got.Active)
}

func TestApplyDeactivation_AlreadyInactiveIsIdempotent(t *testing.T) {
	cur := activeUser("u-d", domain.RoleDriver)
	cur.Active = false

	got, err := domain.ApplyDeactivation(cur, 1, "u-admin")
	require.NoError(t, err)
	assert.False(t, got.Active)
}
