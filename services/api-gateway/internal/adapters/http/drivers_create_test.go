package http_test

import (
	"context"
	"encoding/json"
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

// creatingDrivers responde CreateDriver con el perfil pedido, o con err si se configura.
type creatingDrivers struct {
	fakeDriverClient
	last *driversv1.CreateDriverRequest
	err  error
}

func (f *creatingDrivers) CreateDriver(_ context.Context, _ grpcclient.CallMeta, req *driversv1.CreateDriverRequest) (*driversv1.CreateDriverResponse, error) {
	f.calls++
	f.last = req
	if f.err != nil {
		return nil, f.err
	}
	return &driversv1.CreateDriverResponse{Driver: &driversv1.DriverProfile{
		Id: "drv-1", UserId: req.GetUserId(), FirstName: req.GetFirstName(), LastName: req.GetLastName(),
		BirthDate: req.GetBirthDate(), Phone: req.GetPhone(), VehicleType: req.GetVehicleType(),
		Identifier: req.GetIdentifier(), Plate: "PBA1234", City: req.GetCity(), Active: true,
		Status: driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, CreatedAt: "2026-10-16T10:00:00Z",
	}}, nil
}

const createDriverBody = `{"email":"lucas@example.com","password":"password123","firstName":"Lucas","lastName":"Martinez",
"birthDate":"1999-03-15","phone":"0987654321","vehicleType":"MOTORIZADO","identifier":"1712345678","plate":"PBA-1234","city":"Quito"}`

func adminHeader(t *testing.T) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token(t, "u-admin", "ADMIN")}
}

func createDriverRouter(auth httpapi.AuthClient, drivers httpapi.DriverClient) http.Handler {
	return newRouterWithSignup(auth, httpapi.Options{Drivers: drivers})
}

func TestCreateDriver_OnlyAdmin(t *testing.T) {
	drivers := &creatingDrivers{}
	auth := &fakeAuth{}
	h := createDriverRouter(auth, drivers)

	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, "/api/v1/drivers", createDriverBody, nil).Code)
	for _, role := range []string{"OPERATOR", "DRIVER", "CUSTOMER"} {
		rec := do(h, http.MethodPost, "/api/v1/drivers", createDriverBody, map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)})
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
	}
	assert.Zero(t, auth.createCalls)
	assert.Zero(t, drivers.calls)
}

func TestCreateDriver_CreatesDriverUserAndProfile(t *testing.T) {
	drivers := &creatingDrivers{}
	auth := &fakeAuth{}

	rec := do(createDriverRouter(auth, drivers), http.MethodPost, "/api/v1/drivers", createDriverBody, adminHeader(t))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NotNil(t, auth.lastCreate)
	assert.Equal(t, authv1.Role_ROLE_DRIVER, auth.lastCreate.GetRole())
	assert.Equal(t, "Lucas Martinez", auth.lastCreate.GetFullName())
	assert.Equal(t, "lucas@example.com", auth.lastCreate.GetEmail())
	require.NotNil(t, drivers.last)
	assert.Equal(t, "u-2", drivers.last.GetUserId())
	assert.Equal(t, driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE, drivers.last.GetVehicleType())
	assert.Equal(t, "1999-03-15", drivers.last.GetBirthDate())
	assert.Nil(t, auth.lastDeactivate)

	var body struct {
		User   struct{ ID, Role string } `json:"user"`
		Driver struct {
			ID          string `json:"id"`
			VehicleType string `json:"vehicleType"`
			Plate       string `json:"plate"`
			Status      string `json:"status"`
		} `json:"driver"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "drv-1", body.Driver.ID)
	assert.Equal(t, "MOTORIZADO", body.Driver.VehicleType)
	assert.Equal(t, "PBA1234", body.Driver.Plate)
	assert.Equal(t, "AVAILABLE", body.Driver.Status)
}

func TestCreateDriver_MalformedProfile_Returns400WithoutCreatingUser(t *testing.T) {
	for name, body := range map[string]string{
		"fecha mal formada": `{"email":"a@b.co","password":"password123","firstName":"A","lastName":"B","birthDate":"15/03/1999","vehicleType":"SUV"}`,
		"vehículo inválido": `{"email":"a@b.co","password":"password123","firstName":"A","lastName":"B","birthDate":"1999-03-15","vehicleType":"BICICLETA"}`,
		"json roto":         `{`,
	} {
		auth := &fakeAuth{}
		drivers := &creatingDrivers{}
		rec := do(createDriverRouter(auth, drivers), http.MethodPost, "/api/v1/drivers", body, adminHeader(t))
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
		assert.Zero(t, auth.createCalls, name)
		assert.Zero(t, drivers.calls, name)
	}
}

func TestCreateDriver_UserCreationFails_DoesNotCreateProfile(t *testing.T) {
	auth := &fakeAuth{err: status.Error(codes.AlreadyExists, "email already registered")}
	drivers := &creatingDrivers{}

	rec := do(createDriverRouter(auth, drivers), http.MethodPost, "/api/v1/drivers", createDriverBody, adminHeader(t))

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Zero(t, drivers.calls)
}

func TestCreateDriver_ProfileFails_DeactivatesTheCreatedUser(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"datos inválidos": {status.Error(codes.InvalidArgument, "birth date"), http.StatusBadRequest},
		"duplicado":       {status.Error(codes.AlreadyExists, "plate"), http.StatusConflict},
	} {
		auth := &fakeAuth{}
		drivers := &creatingDrivers{err: tc.err}

		rec := do(createDriverRouter(auth, drivers), http.MethodPost, "/api/v1/drivers", createDriverBody, adminHeader(t))

		assert.Equal(t, tc.want, rec.Code, name)
		require.NotNil(t, auth.lastDeactivate, name)
		assert.Equal(t, "u-2", auth.lastDeactivate.GetId(), name)
	}
}

func TestCreateDriver_CompensationFails_StillReportsTheOriginalError(t *testing.T) {
	auth := &fakeAuth{deactivateErr: status.Error(codes.Unavailable, "auth down")}
	drivers := &creatingDrivers{err: status.Error(codes.InvalidArgument, "plate")}

	rec := do(createDriverRouter(auth, drivers), http.MethodPost, "/api/v1/drivers", createDriverBody, adminHeader(t))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotNil(t, auth.lastDeactivate)
}
