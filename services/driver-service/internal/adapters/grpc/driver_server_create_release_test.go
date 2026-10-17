package grpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
)

// roleCtx simulates the x-user-role identity that the Gateway forwards to Driver Service.
// An empty role means no identity metadata at all.
func roleCtx(role string) context.Context {
	if role == "" {
		return context.Background()
	}
	return metadata.AppendToOutgoingContext(context.Background(), "x-user-role", role)
}

func createRequest() *driversv1.CreateDriverRequest {
	return &driversv1.CreateDriverRequest{UserId: "user-new", FullName: "Ana Ruiz", Phone: "0991234567"}
}

func TestCreateDriverRequiresAdminRole(t *testing.T) {
	for _, role := range []string{"", "OPERATOR", "DRIVER", "CUSTOMER"} {
		t.Run("role "+role, func(t *testing.T) {
			client, repo := setup(t)

			_, err := client.CreateDriver(roleCtx(role), createRequest())

			assert.Equal(t, codes.PermissionDenied, status.Code(err))
			list, err := repo.FindAvailable(context.Background(), 10)
			require.NoError(t, err)
			assert.Empty(t, list, "a refused create stores nothing")
		})
	}
}

func TestCreateDriverAdminCreatesAvailableDriver(t *testing.T) {
	client, _ := setup(t)

	resp, err := client.CreateDriver(roleCtx("ADMIN"), createRequest())

	require.NoError(t, err)
	got := resp.GetDriver()
	assert.NotEmpty(t, got.GetId())
	assert.Equal(t, "user-new", got.GetUserId())
	assert.Equal(t, "Ana Ruiz", got.GetFullName())
	assert.True(t, got.GetActive())
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, got.GetStatus())
}

func TestCreateDriverInvalidInput(t *testing.T) {
	client, _ := setup(t)
	tests := []struct {
		name string
		req  *driversv1.CreateDriverRequest
	}{
		{"blank user_id", &driversv1.CreateDriverRequest{UserId: " ", FullName: "Ana", Phone: "0991234567"}},
		{"blank full_name", &driversv1.CreateDriverRequest{UserId: "u", FullName: "  ", Phone: "0991234567"}},
		{"invalid phone", &driversv1.CreateDriverRequest{UserId: "u", FullName: "Ana", Phone: "no-es-telefono"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateDriver(roleCtx("ADMIN"), tt.req)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestCreateDriverDuplicateUserIsAlreadyExists(t *testing.T) {
	client, _ := setup(t)
	_, err := client.CreateDriver(roleCtx("ADMIN"), createRequest())
	require.NoError(t, err)

	_, err = client.CreateDriver(roleCtx("ADMIN"), createRequest())

	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestReleaseDriverCodigosYRespuesta(t *testing.T) {
	client, repo := setup(t)
	seedDriver(t, repo, "d1")
	ctx := context.Background()
	_, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)

	tests := []struct {
		name     string
		driverID string
		routeID  string
		code     codes.Code
	}{
		{"driver_id blank", " ", "route-1", codes.InvalidArgument},
		{"route_id blank", "d1", "", codes.InvalidArgument},
		{"unknown driver", "nadie", "route-1", codes.NotFound},
		{"reserved for another route", "d1", "route-2", codes.FailedPrecondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: tt.driverID, RouteId: tt.routeID})
			assert.Equal(t, tt.code, status.Code(err))
		})
	}

	resp, err := client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err)
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, resp.GetDriver().GetStatus())

	again, err := client.ReleaseDriver(ctx, &driversv1.ReleaseDriverRequest{DriverId: "d1", RouteId: "route-1"})
	require.NoError(t, err, "a repeated release is idempotent")
	assert.Equal(t, driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE, again.GetDriver().GetStatus())
}
