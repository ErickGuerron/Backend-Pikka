package grpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
)

func TestUpdateUser(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	driver, err := c.CreateUser(asCaller("u-1", "ADMIN"), &authv1.CreateUserRequest{Email: "driver@example.com", Password: "password123", FullName: "Driver", Role: authv1.Role_ROLE_DRIVER})
	require.NoError(t, err)
	id := driver.GetUser().GetId()

	name := "Driver Renombrado"
	role := authv1.Role_ROLE_OPERATOR
	got, err := c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: id, FullName: &name, Role: &role})
	require.NoError(t, err)
	assert.Equal(t, name, got.GetUser().GetFullName())
	assert.Equal(t, authv1.Role_ROLE_OPERATOR, got.GetUser().GetRole())

	// Solo el nombre: el rol no cambia.
	got, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: id, FullName: &name})
	require.NoError(t, err)
	assert.Equal(t, authv1.Role_ROLE_OPERATOR, got.GetUser().GetRole())

	_, err = c.UpdateUser(asCaller("u-7", "DRIVER"), &authv1.UpdateUserRequest{Id: id, FullName: &name})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.UpdateUser(ctx, &authv1.UpdateUserRequest{Id: id, FullName: &name})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "sin identidad propagada")

	_, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: "u-404", FullName: &name})
	assert.Equal(t, codes.NotFound, status.Code(err))

	customer := authv1.Role_ROLE_CUSTOMER
	_, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: id, Role: &customer})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "CUSTOMER no se asigna")

	unspecified := authv1.Role_ROLE_UNSPECIFIED
	_, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: id, Role: &unspecified})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: "u-1", Role: &role})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "último ADMIN activo no se degrada")
}

func TestDeactivateUser(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	driver, err := c.CreateUser(asCaller("u-1", "ADMIN"), &authv1.CreateUserRequest{Email: "driver@example.com", Password: "password123", FullName: "Driver", Role: authv1.Role_ROLE_DRIVER})
	require.NoError(t, err)
	id := driver.GetUser().GetId()

	_, err = c.DeactivateUser(asCaller("u-7", "DRIVER"), &authv1.DeactivateUserRequest{Id: id})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.DeactivateUser(ctx, &authv1.DeactivateUserRequest{Id: id})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = c.DeactivateUser(asCaller("u-1", "ADMIN"), &authv1.DeactivateUserRequest{Id: "u-1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "no puede desactivarse a sí mismo")

	got, err := c.DeactivateUser(asCaller("u-1", "ADMIN"), &authv1.DeactivateUserRequest{Id: id})
	require.NoError(t, err)
	assert.False(t, got.GetUser().GetActive())

	_, err = c.Login(ctx, &authv1.LoginRequest{Email: "driver@example.com", Password: "password123"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "un usuario inactivo no entra")
}
