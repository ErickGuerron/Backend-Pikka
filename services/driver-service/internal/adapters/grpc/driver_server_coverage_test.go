package grpc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// covDrivers envuelve el repositorio en memoria y falla la lista cuando se configura.
type covDrivers struct {
	*memory.DriverRepository
	listErr error
}

func (r covDrivers) List(ctx context.Context, limit int) ([]*domain.Driver, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.DriverRepository.List(ctx, limit)
}

// covCreate arma una solicitud de alta válida con un sufijo que la hace única.
func covCreate(suffix string, vehicle driversv1.VehicleType) *driversv1.CreateDriverRequest {
	return &driversv1.CreateDriverRequest{
		UserId:      "user-" + suffix,
		FirstName:   "Ana",
		LastName:    "Ruiz",
		BirthDate:   "1995-05-20",
		Phone:       "0991234567",
		VehicleType: vehicle,
		Identifier:  "ID" + suffix,
		Plate:       "PL" + suffix,
		City:        "Quito",
	}
}

func TestListDriversMapsVehicleAndStatusForEveryState(t *testing.T) {
	repo := memory.NewDriverRepository()
	svc := driver.NewService(repo, func() time.Time { return t0 })
	client := serve(t, svc)
	ctx := context.Background()

	ids := map[string]string{}
	for suffix, vehicle := range map[string]driversv1.VehicleType{
		"0001": driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE,
		"0002": driversv1.VehicleType_VEHICLE_TYPE_CAR,
		"0003": driversv1.VehicleType_VEHICLE_TYPE_SUV,
		"0004": driversv1.VehicleType_VEHICLE_TYPE_CAR,
	} {
		created, err := client.CreateDriver(ctx, covCreate(suffix, vehicle))
		require.NoError(t, err)
		ids[suffix] = created.GetDriver().GetId()
	}
	// Un repartidor en cada estado: disponible, asignado, en ruta y fuera de línea.
	_, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: ids["0002"], RouteId: "route-1"})
	require.NoError(t, err)
	_, err = client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: ids["0003"], RouteId: "route-2"})
	require.NoError(t, err)
	_, err = client.StartRoute(ctx, &driversv1.StartRouteRequest{DriverId: ids["0003"], RouteId: "route-2"})
	require.NoError(t, err)
	_, err = svc.GoOffline(ctx, ids["0004"])
	require.NoError(t, err)

	res, err := client.ListDrivers(ctx, &driversv1.ListDriversRequest{})

	require.NoError(t, err)
	require.Len(t, res.GetDrivers(), 4)
	byID := map[string]*driversv1.DriverListItem{}
	for _, d := range res.GetDrivers() {
		byID[d.GetId()] = d
	}
	assert.Equal(t, driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE, byID[ids["0001"]].GetVehicleType())
	assert.Equal(t, driversv1.VehicleType_VEHICLE_TYPE_CAR, byID[ids["0002"]].GetVehicleType())
	assert.Equal(t, driversv1.VehicleType_VEHICLE_TYPE_SUV, byID[ids["0003"]].GetVehicleType())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, byID[ids["0001"]].GetStatus())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_ASSIGNED, byID[ids["0002"]].GetStatus())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_ON_ROUTE, byID[ids["0003"]].GetStatus())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_OFFLINE, byID[ids["0004"]].GetStatus())
	assert.Equal(t, "PL0001", byID[ids["0001"]].GetPlate())
}

func TestListDriversRepositoryFailureIsInternal(t *testing.T) {
	svc := driver.NewService(covDrivers{DriverRepository: memory.NewDriverRepository(), listErr: errors.New("db down")}, func() time.Time { return t0 })

	_, err := serve(t, svc).ListDrivers(context.Background(), &driversv1.ListDriversRequest{})

	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, "internal error", status.Convert(err).Message())
}

func TestCreateDriverRejectsUnspecifiedVehicle(t *testing.T) {
	client, _ := setup(t)

	_, err := client.CreateDriver(context.Background(), covCreate("0009", driversv1.VehicleType_VEHICLE_TYPE_UNSPECIFIED))

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateDriverRejectsInvalidProfileFields(t *testing.T) {
	client, _ := setup(t)
	ctx := context.Background()
	motorbike := driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE

	tests := map[string]func(*driversv1.CreateDriverRequest){
		"blank user id":      func(r *driversv1.CreateDriverRequest) { r.UserId = "" },
		"blank first name":   func(r *driversv1.CreateDriverRequest) { r.FirstName = "" },
		"blank last name":    func(r *driversv1.CreateDriverRequest) { r.LastName = "" },
		"malformed phone":    func(r *driversv1.CreateDriverRequest) { r.Phone = "1" },
		"short identifier":   func(r *driversv1.CreateDriverRequest) { r.Identifier = "x" },
		"short plate":        func(r *driversv1.CreateDriverRequest) { r.Plate = "x" },
		"blank city":         func(r *driversv1.CreateDriverRequest) { r.City = "" },
		"underage driver":    func(r *driversv1.CreateDriverRequest) { r.BirthDate = "2015-01-01" },
		"birth date not ISO": func(r *driversv1.CreateDriverRequest) { r.BirthDate = "20/05/1995" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := covCreate("0010", motorbike)
			mutate(req)
			_, err := client.CreateDriver(ctx, req)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestDriverServerRejectsBlankIdentifiers(t *testing.T) {
	client, _ := setup(t)
	ctx := context.Background()

	_, err := client.GetDriverByUserID(ctx, &driversv1.GetDriverByUserIDRequest{UserId: "   "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: " "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = client.StartRoute(ctx, &driversv1.StartRouteRequest{DriverId: "", RouteId: "route-1"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: "d1", RouteId: ""})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestFindAvailableDriversNegativeLimitIsInvalidAndDefaultLimitWorks(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	ctx := context.Background()

	res, err := client.FindAvailableDrivers(ctx, &driversv1.FindAvailableDriversRequest{Limit: 0})
	require.NoError(t, err)
	assert.Len(t, res.GetDrivers(), 1, "limit 0 usa el valor por defecto")

	_, err = client.FindAvailableDrivers(ctx, &driversv1.FindAvailableDriversRequest{Limit: -5})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
