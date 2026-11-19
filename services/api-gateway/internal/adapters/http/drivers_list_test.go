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

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// listingDrivers devuelve una flota fija, o err si se configura.
type listingDrivers struct {
	fakeDriverClient
	err error
}

func (f *listingDrivers) ListDrivers(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &driversv1.ListDriversResponse{Drivers: []*driversv1.DriverListItem{
		{Id: "d-1", UserId: "u-1", FirstName: "Lucas", LastName: "Martinez", Phone: "0987654321", City: "Quito",
			VehicleType: driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE, Plate: "PBA1234", Active: true,
			Status: driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE},
		{Id: "d-2", UserId: "u-2", FirstName: "Ana", LastName: "Ruiz", Phone: "0991112233", City: "Cuenca",
			VehicleType: driversv1.VehicleType_VEHICLE_TYPE_SUV, Plate: "ABC123", Active: false,
			Status: driversv1.DriverStatus_DRIVER_STATUS_OFFLINE},
	}}, nil
}

func TestListDrivers_OnlyStaff(t *testing.T) {
	drivers := &listingDrivers{}
	h := createDriverRouter(&fakeAuth{}, drivers)

	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/api/v1/drivers", "", nil).Code)
	for _, role := range []string{"DRIVER", "CUSTOMER"} {
		rec := do(h, http.MethodGet, "/api/v1/drivers", "", map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)})
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
	}
	assert.Zero(t, drivers.calls)
}

func TestListDrivers_StaffGetsTheFleetWithoutPersonalData(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR"} {
		h := createDriverRouter(&fakeAuth{}, &listingDrivers{})

		rec := do(h, http.MethodGet, "/api/v1/drivers", "", map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)})

		require.Equal(t, http.StatusOK, rec.Code, role)
		var list []map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list), role)
		require.Len(t, list, 2, role)
		first := list[0]
		assert.Equal(t, "Lucas Martinez", first["fullName"])
		assert.Equal(t, "Lucas", first["firstName"])
		assert.Equal(t, "Quito", first["city"])
		assert.Equal(t, "MOTORIZADO", first["vehicleType"])
		assert.Equal(t, "PBA1234", first["plate"])
		assert.Equal(t, "AVAILABLE", first["status"])
		assert.Equal(t, true, first["active"])
		assert.Equal(t, "SUV", list[1]["vehicleType"])
		assert.Equal(t, false, list[1]["active"])
		assert.NotContains(t, first, "birthDate")
		assert.NotContains(t, first, "identifier")
	}
}

func TestListDrivers_EmptyFleetIsAnEmptyArray(t *testing.T) {
	h := createDriverRouter(&fakeAuth{}, &emptyFleet{})
	rec := do(h, http.MethodGet, "/api/v1/drivers", "", adminHeader(t))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[]`, rec.Body.String())
}

type emptyFleet struct{ fakeDriverClient }

func (f *emptyFleet) ListDrivers(context.Context, grpcclient.CallMeta, *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	return &driversv1.ListDriversResponse{}, nil
}

func TestListDrivers_UpstreamFailureIsMapped(t *testing.T) {
	h := createDriverRouter(&fakeAuth{}, &listingDrivers{err: status.Error(codes.Unavailable, "driver-service down")})
	rec := do(h, http.MethodGet, "/api/v1/drivers", "", adminHeader(t))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
