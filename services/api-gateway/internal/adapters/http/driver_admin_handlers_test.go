package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

const (
	driversPath     = "/api/v1/drivers"
	driverUserID    = "6f1d2c3a-8b4e-4f5a-9c7d-1e2f3a4b5c6d"
	validDriverBody = `{"userId":"` + driverUserID + `","fullName":"Ana Lopez","phone":"+593991234567"}`
)

// userLookupAuth answers GetUser with the configured role and counts lookups.
type userLookupAuth struct {
	*fakeAuth
	role     authv1.Role
	inactive bool
	err      error
	lookups  int
	lastID   string
}

func (u *userLookupAuth) GetUser(_ context.Context, _ grpcclient.CallMeta, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error) {
	u.lookups++
	u.lastID = req.GetId()
	if u.err != nil {
		return nil, u.err
	}
	return &authv1.GetUserResponse{User: &authv1.User{Id: req.GetId(), Role: u.role, Active: !u.inactive}}, nil
}

// fakeDriverCreator counts creations and records the request and identity it received.
type fakeDriverCreator struct {
	calls    int
	lastMeta grpcclient.CallMeta
	lastReq  *driversv1.CreateDriverRequest
	err      error
}

func (f *fakeDriverCreator) CreateDriver(_ context.Context, m grpcclient.CallMeta, req *driversv1.CreateDriverRequest) (*driversv1.CreateDriverResponse, error) {
	f.calls++
	f.lastMeta = m
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return &driversv1.CreateDriverResponse{Driver: &driversv1.Driver{
		Id: "d-9", UserId: req.GetUserId(), FullName: req.GetFullName(), Phone: req.GetPhone(),
		Active: true, Status: driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, CreatedAt: "2026-10-16T12:00:00Z",
	}}, nil
}

func driverAdminRouter(auth httpapi.AuthClient, creator httpapi.DriverCreator) http.Handler {
	return newRouterWithSignup(auth, httpapi.Options{DriverCreator: creator})
}

func TestCreateDriver_WithoutToken_Returns401WithoutCall(t *testing.T) {
	auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER}
	creator := &fakeDriverCreator{}
	h := driverAdminRouter(auth, creator)

	rec := do(h, http.MethodPost, driversPath, validDriverBody, nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, auth.lookups)
	assert.Zero(t, creator.calls)
}

// Only ADMIN may create drivers. DRIVER, OPERATOR and CUSTOMER get 403 before any lookup or create.
func TestCreateDriver_NonAdminRoles_Returns403WithoutCall(t *testing.T) {
	auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER}
	creator := &fakeDriverCreator{}
	h := driverAdminRouter(auth, creator)

	for _, role := range []string{"DRIVER", "OPERATOR", "CUSTOMER"} {
		rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code, role)
	}
	assert.Zero(t, auth.lookups)
	assert.Zero(t, creator.calls)
}

func TestCreateDriver_AdminWithDriverUser_Returns201WithDriver(t *testing.T) {
	auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER}
	creator := &fakeDriverCreator{}
	h := driverAdminRouter(auth, creator)

	rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.JSONEq(t, `{"id":"d-9","userId":"`+driverUserID+`","fullName":"Ana Lopez","phone":"+593991234567",
		"active":true,"status":"AVAILABLE","createdAt":"2026-10-16T12:00:00Z"}`, rec.Body.String())

	// The DRIVER role is checked before the create, using the userId from the body.
	assert.Equal(t, 1, auth.lookups)
	assert.Equal(t, driverUserID, auth.lastID)
	require.Equal(t, 1, creator.calls)
	assert.Equal(t, driverUserID, creator.lastReq.GetUserId())
	assert.Equal(t, "Ana Lopez", creator.lastReq.GetFullName())
	assert.Equal(t, "+593991234567", creator.lastReq.GetPhone())
	assert.Equal(t, "ADMIN", creator.lastMeta.Role, "the admin identity is forwarded")
}

func TestCreateDriver_InvalidBody_Returns400WithoutCall(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"userId missing", `{"fullName":"Ana Lopez","phone":"+593991234567"}`, "userId is required"},
		{"userId blank", `{"userId":"   ","fullName":"Ana Lopez","phone":"+593991234567"}`, "userId is required"},
		{"fullName missing", `{"userId":"` + driverUserID + `","phone":"+593991234567"}`, "fullName is required"},
		{"phone missing", `{"userId":"` + driverUserID + `","fullName":"Ana Lopez"}`, "phone is required"},
		{"malformed JSON", `{"userId":`, "malformed JSON body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER}
			creator := &fakeDriverCreator{}
			h := driverAdminRouter(auth, creator)

			rec := do(h, http.MethodPost, driversPath, tc.body, authHeaderFor(t, "ADMIN"))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			e := decodeError(t, rec)
			assert.Equal(t, "INVALID_ARGUMENT", e.Code)
			assert.Equal(t, tc.message, e.Message)
			assert.Zero(t, auth.lookups)
			assert.Zero(t, creator.calls)
		})
	}
}

// A user that exists but is not a DRIVER gets no profile: 422 FAILED_PRECONDITION, no create call.
func TestCreateDriver_UserWithoutDriverRole_Returns422WithoutCreating(t *testing.T) {
	for _, role := range []authv1.Role{authv1.Role_ROLE_ADMIN, authv1.Role_ROLE_OPERATOR, authv1.Role_ROLE_CUSTOMER} {
		auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: role}
		creator := &fakeDriverCreator{}
		h := driverAdminRouter(auth, creator)

		rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, role.String())
		assert.Equal(t, "FAILED_PRECONDITION", decodeError(t, rec).Code, role.String())
		assert.Equal(t, 1, auth.lookups, role.String())
		assert.Zero(t, creator.calls, role.String())
	}
}

// A DRIVER-role user that Auth reports as inactive gets no profile. 422 FAILED_PRECONDITION, the same
// mapping as a wrong role: the user exists but is not eligible to become a driver. No create call.
func TestCreateDriver_InactiveDriverUser_Returns422WithoutCreating(t *testing.T) {
	auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER, inactive: true}
	creator := &fakeDriverCreator{}
	h := driverAdminRouter(auth, creator)

	rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "FAILED_PRECONDITION", decodeError(t, rec).Code)
	assert.Equal(t, 1, auth.lookups)
	assert.Zero(t, creator.calls)
}

func TestCreateDriver_UnknownUser_Returns404WithoutCreating(t *testing.T) {
	auth := &userLookupAuth{fakeAuth: &fakeAuth{}, err: status.Error(codes.NotFound, "user not found")}
	creator := &fakeDriverCreator{}
	h := driverAdminRouter(auth, creator)

	rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "NOT_FOUND", decodeError(t, rec).Code)
	assert.Zero(t, creator.calls)
}

// If Auth does not answer, the role cannot be verified, so no driver is created.
func TestCreateDriver_AuthUnavailable_Returns503WithoutCreating(t *testing.T) {
	auth := &userLookupAuth{fakeAuth: &fakeAuth{}, err: status.Error(codes.Unavailable, "auth unavailable")}
	creator := &fakeDriverCreator{}
	h := driverAdminRouter(auth, creator)

	rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "UPSTREAM_UNAVAILABLE", decodeError(t, rec).Code)
	assert.Zero(t, creator.calls)
}

// ALREADY_EXISTS from Driver Service (the user already has a profile) maps to 409 CONFLICT.
func TestCreateDriver_DriverServiceErrors_MapToErrorEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantName string
	}{
		{"user already has a driver profile", status.Error(codes.AlreadyExists, "driver already exists"), http.StatusConflict, "CONFLICT"},
		{"driver service rejects the data", status.Error(codes.InvalidArgument, "phone is invalid"), http.StatusBadRequest, "INVALID_ARGUMENT"},
		{"driver service unavailable", status.Error(codes.Unavailable, "driver unavailable"), http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auth := &userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER}
			creator := &fakeDriverCreator{err: tc.err}
			h := driverAdminRouter(auth, creator)

			rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

			assert.Equal(t, tc.wantCode, rec.Code)
			assert.Equal(t, tc.wantName, decodeError(t, rec).Code)
			assert.Equal(t, 1, creator.calls)
		})
	}
}

// Without the Driver Service client the route does not exist.
func TestCreateDriver_RouteAbsentWithoutDriverCreator(t *testing.T) {
	h := newRouterWithSignup(&userLookupAuth{fakeAuth: &fakeAuth{}, role: authv1.Role_ROLE_DRIVER}, httpapi.Options{})

	rec := do(h, http.MethodPost, driversPath, validDriverBody, authHeaderFor(t, "ADMIN"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
