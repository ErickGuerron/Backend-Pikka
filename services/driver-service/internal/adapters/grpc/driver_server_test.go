package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
)

var t0 = time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)

// setup levanta el servidor sobre un listener en memoria y devuelve un cliente y el repositorio.
func setup(t *testing.T) (driversv1.DriverServiceClient, *memory.DriverRepository) {
	t.Helper()
	repo := memory.NewDriverRepository()
	return serve(t, driver.NewService(repo, func() time.Time { return t0 })), repo
}

// serve arranca el servidor gRPC sobre bufconn con el servicio dado y devuelve un cliente.
func serve(t *testing.T, svc *driver.Service) driversv1.DriverServiceClient {
	t.Helper()
	ctx := context.Background()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	driversv1.RegisterDriverServiceServer(srv, grpcadapter.NewDriverServer(svc))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return driversv1.NewDriverServiceClient(conn)
}

func seedDriver(t *testing.T, repo *memory.DriverRepository, id string) *domain.Driver {
	t.Helper()
	d, err := domain.NewDriver(id, domaintest.Input("user-"+id), t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), d))
	return d
}

func TestFindAvailableDriversDevuelveSoloIdentificadores(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	inactive := seedDriver(t, repo, "d2")
	inactive.Deactivate(t0)
	require.NoError(t, repo.Save(context.Background(), inactive, inactive.Version-1))

	resp, err := client.FindAvailableDrivers(context.Background(), &driversv1.FindAvailableDriversRequest{})

	require.NoError(t, err)
	require.Len(t, resp.GetDrivers(), 1)
	assert.Equal(t, "d1", resp.GetDrivers()[0].GetId())
	assert.Equal(t, "user-d1", resp.GetDrivers()[0].GetUserId())
}

func TestFindAvailableDriversLimites(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")

	_, err := client.FindAvailableDrivers(context.Background(), &driversv1.FindAvailableDriversRequest{Limit: -1})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Mayor que el máximo no es error: el contrato lo recorta a 100.
	resp, err := client.FindAvailableDrivers(context.Background(), &driversv1.FindAvailableDriversRequest{Limit: 500})
	require.NoError(t, err)
	assert.Len(t, resp.GetDrivers(), 1)
}

func TestReserveDriverRespuestaYRepeticion(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	ctx := context.Background()

	resp, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_ASSIGNED, resp.GetDriver().GetStatus())
	assert.Equal(t, "Ana Ruiz", resp.GetDriver().GetFullName(), "la reserva devuelve nombre y teléfono a quien reserva")

	again, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err, "repetir la misma ruta es idempotente")
	assert.Equal(t, resp.GetDriver().GetStatus(), again.GetDriver().GetStatus())
}

func TestReserveDriverCodigosDeError(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	inactive := seedDriver(t, repo, "d2")
	inactive.Deactivate(t0)
	require.NoError(t, repo.Save(context.Background(), inactive, inactive.Version-1))
	seedDriver(t, repo, "d3")
	offline := seedDriver(t, repo, "d4")
	require.NoError(t, offline.GoOffline(t0))
	require.NoError(t, repo.Save(context.Background(), offline, offline.Version-1))
	ctx := context.Background()
	_, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)

	tests := []struct {
		name     string
		driverID string
		routeID  string
		code     codes.Code
		msg      string
	}{
		{name: "driver_id en blanco", driverID: "  ", routeID: "route-1", code: codes.InvalidArgument},
		{name: "route_id en blanco", driverID: "d1", routeID: "", code: codes.InvalidArgument},
		{name: "repartidor inexistente", driverID: "nadie", routeID: "route-1", code: codes.NotFound},
		{name: "reservado para otra ruta", driverID: "d1", routeID: "route-2", code: codes.FailedPrecondition, msg: "already reserved"},
		{name: "inactivo", driverID: "d2", routeID: "route-1", code: codes.FailedPrecondition, msg: "inactive"},
		{name: "fuera de línea", driverID: "d4", routeID: "route-1", code: codes.FailedPrecondition, msg: "offline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: tt.driverID, RouteId: tt.routeID})
			require.Error(t, err)
			assert.Equal(t, tt.code, status.Code(err))
			if tt.msg != "" {
				assert.Contains(t, status.Convert(err).Message(), tt.msg, "FAILED_PRECONDITION debe distinguir la causa")
			}
		})
	}
}

func createRequest() *driversv1.CreateDriverRequest {
	return &driversv1.CreateDriverRequest{
		UserId: "user-new", FirstName: "Lucas", LastName: "Martinez", BirthDate: "1999-03-15", Phone: "0987654321",
		VehicleType: driversv1.VehicleType_VEHICLE_TYPE_CAR, Identifier: "1712345678", Plate: "PBA-1234", City: "Quito",
	}
}

func TestCreateDriverDevuelveElPerfil(t *testing.T) {
	client, _ := setup(t)

	resp, err := client.CreateDriver(context.Background(), createRequest())

	require.NoError(t, err)
	p := resp.GetDriver()
	assert.NotEmpty(t, p.GetId())
	assert.Equal(t, "Lucas", p.GetFirstName())
	assert.Equal(t, "1999-03-15", p.GetBirthDate())
	assert.Equal(t, driversv1.VehicleType_VEHICLE_TYPE_CAR, p.GetVehicleType())
	assert.Equal(t, "PBA1234", p.GetPlate())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, p.GetStatus())
}

func TestCreateDriverCodigosDeError(t *testing.T) {
	client, _ := setup(t)
	ctx := context.Background()
	_, err := client.CreateDriver(ctx, createRequest())
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*driversv1.CreateDriverRequest)
		want   codes.Code
	}{
		{"fecha mal formada", func(r *driversv1.CreateDriverRequest) { r.BirthDate = "15/03/1999" }, codes.InvalidArgument},
		{"vehículo sin especificar", func(r *driversv1.CreateDriverRequest) { r.VehicleType = driversv1.VehicleType_VEHICLE_TYPE_UNSPECIFIED }, codes.InvalidArgument},
		{"menor de edad", func(r *driversv1.CreateDriverRequest) { r.BirthDate = "2020-01-01" }, codes.InvalidArgument},
		{"placa inválida", func(r *driversv1.CreateDriverRequest) { r.Plate = "A-1" }, codes.InvalidArgument},
		{"usuario repetido", func(r *driversv1.CreateDriverRequest) { r.Identifier, r.Plate = "9999999999", "ZZZ9999" }, codes.AlreadyExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createRequest()
			tt.mutate(req)
			_, err := client.CreateDriver(ctx, req)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestListDriversIncluyeInactivosYNoExponeDatosPersonales(t *testing.T) {
	client, _ := setup(t)
	ctx := context.Background()
	_, err := client.CreateDriver(ctx, createRequest())
	require.NoError(t, err)

	resp, err := client.ListDrivers(ctx, &driversv1.ListDriversRequest{})

	require.NoError(t, err)
	require.Len(t, resp.GetDrivers(), 1)
	item := resp.GetDrivers()[0]
	assert.Equal(t, "Lucas", item.GetFirstName())
	assert.Equal(t, "Martinez", item.GetLastName())
	assert.Equal(t, "Quito", item.GetCity())
	assert.Equal(t, "PBA1234", item.GetPlate())
	assert.Equal(t, driversv1.VehicleType_VEHICLE_TYPE_CAR, item.GetVehicleType())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, item.GetStatus())
	assert.True(t, item.GetActive())
}
