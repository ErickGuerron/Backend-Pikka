package http_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
)

// --- PATCH y DELETE /api/v1/users/:id (solo ADMIN) ---------------------------

func TestUpdateUser_WithoutAuth_Returns401(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodPatch, "/api/v1/users/u-7", `{"fullName":"X"}`, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "UNAUTHENTICATED", decodeError(t, rec).Code)
}

func TestUpdateUser_NonAdminRoles_Return403WithoutCallingAuth(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "OPERATOR", "DRIVER"} {
		t.Run(role, func(t *testing.T) {
			auth := &fakeAuth{}
			h := newRouter(auth, true)
			rec := do(h, http.MethodPatch, "/api/v1/users/u-7", `{"fullName":"X"}`,
				map[string]string{"Authorization": "Bearer " + token(t, "u-9", role)})
			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code)
			assert.Nil(t, auth.lastUpdate, "el middleware corta antes del servicio")
		})
	}
}

func TestUpdateUser_AsAdmin_ForwardsFieldsAndReturns200(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)
	rec := do(h, http.MethodPatch, "/api/v1/users/u-7", `{"fullName":"Nombre Nuevo","role":"OPERATOR"}`,
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	require.Equal(t, http.StatusOK, rec.Code)

	require.NotNil(t, auth.lastUpdate)
	assert.Equal(t, "u-7", auth.lastUpdate.GetId(), "el id sale de la ruta")
	require.NotNil(t, auth.lastUpdate.FullName)
	assert.Equal(t, "Nombre Nuevo", auth.lastUpdate.GetFullName())
	require.NotNil(t, auth.lastUpdate.Role)
	assert.Equal(t, authv1.Role_ROLE_OPERATOR, auth.lastUpdate.GetRole())
	assert.Equal(t, "u-1", auth.lastMeta.UserID)
	assert.Equal(t, "ADMIN", auth.lastMeta.Role)
	assert.Contains(t, rec.Body.String(), `"id":"u-7"`)
}

func TestUpdateUser_OmittedFields_AreNotForwarded(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)
	rec := do(h, http.MethodPatch, "/api/v1/users/u-7", `{"fullName":"Solo Nombre"}`,
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, auth.lastUpdate.FullName)
	assert.Nil(t, auth.lastUpdate.Role, "un rol ausente no se envía: no cambia")
}

func TestUpdateUser_InvalidRoleFromClient_Returns400(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodPatch, "/api/v1/users/u-7", `{"role":"CUSTOMER"}`,
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "INVALID_ARGUMENT", decodeError(t, rec).Code)
}

func TestUpdateUser_BusinessRuleFromAuth_Returns422(t *testing.T) {
	h := newRouter(&fakeAuth{err: status.Error(codes.FailedPrecondition, "at least one active admin must remain")}, true)
	rec := do(h, http.MethodPatch, "/api/v1/users/u-1", `{"role":"DRIVER"}`,
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "FAILED_PRECONDITION", decodeError(t, rec).Code)
}

func TestDeleteUser_WithoutAuth_Returns401(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodDelete, "/api/v1/users/u-7", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDeleteUser_NonAdminRoles_Return403(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "OPERATOR", "DRIVER"} {
		t.Run(role, func(t *testing.T) {
			auth := &fakeAuth{}
			h := newRouter(auth, true)
			rec := do(h, http.MethodDelete, "/api/v1/users/u-7", "",
				map[string]string{"Authorization": "Bearer " + token(t, "u-9", role)})
			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Nil(t, auth.lastDeactivate)
		})
	}
}

func TestDeleteUser_AsAdmin_Returns200WithInactiveUser(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)
	rec := do(h, http.MethodDelete, "/api/v1/users/u-7", "",
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, auth.lastDeactivate)
	assert.Equal(t, "u-7", auth.lastDeactivate.GetId())
	assert.Equal(t, "u-1", auth.lastMeta.UserID)
	assert.Contains(t, rec.Body.String(), `"active":false`)
}

func TestDeleteUser_NotFoundFromAuth_Returns404(t *testing.T) {
	h := newRouter(&fakeAuth{err: status.Error(codes.NotFound, "user not found")}, true)
	rec := do(h, http.MethodDelete, "/api/v1/users/u-404", "",
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "NOT_FOUND", decodeError(t, rec).Code)
}
