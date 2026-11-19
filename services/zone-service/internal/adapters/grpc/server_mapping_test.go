package grpc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// errRepo falla las operaciones cuyo error se configura y delega el resto en memRepo.
type errRepo struct {
	*memRepo
	saveErr, nextErr, listErr, findErr, containingErr error
}

func (r *errRepo) Save(ctx context.Context, z domain.Zone) (domain.Zone, error) {
	if r.saveErr != nil {
		return domain.Zone{}, r.saveErr
	}
	return r.memRepo.Save(ctx, z)
}

func (r *errRepo) NextCode(ctx context.Context, prefix string) (int, error) {
	if r.nextErr != nil {
		return 0, r.nextErr
	}
	return r.memRepo.NextCode(ctx, prefix)
}

func (r *errRepo) ListAll(ctx context.Context) ([]domain.Zone, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.memRepo.ListAll(ctx)
}

func (r *errRepo) FindByID(ctx context.Context, id string) (domain.Zone, error) {
	if r.findErr != nil {
		return domain.Zone{}, r.findErr
	}
	return r.memRepo.FindByID(ctx, id)
}

func (r *errRepo) FindContaining(ctx context.Context, p domain.Coordinate) (domain.Zone, error) {
	if r.containingErr != nil {
		return domain.Zone{}, r.containingErr
	}
	return r.memRepo.FindContaining(ctx, p)
}

func newErrServer(repo *errRepo) *grpcadapter.Server {
	if repo.memRepo == nil {
		repo.memRepo = &memRepo{zones: map[string]domain.Zone{}}
	}
	return grpcadapter.NewServer(application.NewZoneService(repo))
}

// donutRing es un cuadrado de 4x4 con un hueco centrado de 2x2, en el formato del contrato gRPC.
func donutRing() ([]*zonesv1.Coordinate, []*zonesv1.Ring) {
	outer := []*zonesv1.Coordinate{
		{Latitude: -0.20, Longitude: -78.50},
		{Latitude: -0.20, Longitude: -78.40},
		{Latitude: -0.10, Longitude: -78.40},
		{Latitude: -0.10, Longitude: -78.50},
		{Latitude: -0.20, Longitude: -78.50},
	}
	hole := &zonesv1.Ring{Points: []*zonesv1.Coordinate{
		{Latitude: -0.19, Longitude: -78.49},
		{Latitude: -0.19, Longitude: -78.41},
		{Latitude: -0.11, Longitude: -78.41},
		{Latitude: -0.11, Longitude: -78.49},
		{Latitude: -0.19, Longitude: -78.49},
	}}
	return outer, []*zonesv1.Ring{hole}
}

func TestListZones_ReturnsRegisteredZonesForAdmin(t *testing.T) {
	srv := newServer()
	_, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	require.NoError(t, err)

	res, err := srv.ListZones(asRole("ADMIN"), &zonesv1.ListZonesRequest{})

	require.NoError(t, err)
	require.Len(t, res.GetZones(), 1)
	assert.Equal(t, "NORTE", res.GetZones()[0].GetCode())
	assert.Equal(t, "Hub Norte", res.GetZones()[0].GetSettings().GetDepotName())
}

func TestListZones_RequiresIdentityAndWrapsRepositoryErrors(t *testing.T) {
	_, err := newServer().ListZones(context.Background(), &zonesv1.ListZonesRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	srv := newErrServer(&errRepo{listErr: errors.New("db down")})
	_, err = srv.ListZones(asRole("ADMIN"), &zonesv1.ListZonesRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, "internal error", status.Convert(err).Message(), "no se filtra el detalle de la base de datos")
}

func TestListCoverageZones_RepositoryErrorIsInternal(t *testing.T) {
	srv := newErrServer(&errRepo{listErr: errors.New("db down")})

	_, err := srv.ListCoverageZones(asRole("CUSTOMER"), &zonesv1.ListCoverageZonesRequest{})

	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestCreateZone_RequiresIdentityAndMapsDomainErrors(t *testing.T) {
	outer, _ := donutRing()
	tests := []struct {
		name string
		ctx  context.Context
		req  *zonesv1.CreateZoneRequest
		want codes.Code
	}{
		{
			name: "no identity is unauthenticated",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-role", "ADMIN")),
			req:  &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()},
			want: codes.Unauthenticated,
		},
		{
			name: "invalid name",
			ctx:  asRole("ADMIN"),
			req:  &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "@@", Ring: validRing(), Settings: validProtoSettings()},
			want: codes.InvalidArgument,
		},
		{
			name: "code longer than 32 characters",
			ctx:  asRole("ADMIN"),
			req:  &zonesv1.CreateZoneRequest{Code: "CODIGO-DEMASIADO-LARGO-PARA-LA-ZONA-X", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()},
			want: codes.InvalidArgument,
		},
		{
			name: "coordinate out of range",
			ctx:  asRole("ADMIN"),
			req: &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Settings: validProtoSettings(), Ring: []*zonesv1.Coordinate{
				{Latitude: 200, Longitude: -78.5}, {Latitude: -0.2, Longitude: -78.4}, {Latitude: -0.1, Longitude: -78.4}, {Latitude: -0.1, Longitude: -78.5}, {Latitude: 200, Longitude: -78.5},
			}},
			want: codes.InvalidArgument,
		},
		{
			name: "hole outside outer ring",
			ctx:  asRole("ADMIN"),
			req: &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: outer, Settings: validProtoSettings(), Holes: []*zonesv1.Ring{
				{Points: []*zonesv1.Coordinate{{Latitude: 1, Longitude: 1}, {Latitude: 1, Longitude: 2}, {Latitude: 2, Longitude: 2}, {Latitude: 2, Longitude: 1}, {Latitude: 1, Longitude: 1}}},
			}},
			want: codes.InvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newServer().CreateZone(tt.ctx, tt.req)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestCreateZone_WithHoleReturnsHolesInResponse(t *testing.T) {
	outer, holes := donutRing()

	res, err := newServer().CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{
		Code: "DONA", Name: "Zona Dona", Ring: outer, Holes: holes, Settings: validProtoSettings(),
	})

	require.NoError(t, err)
	require.Len(t, res.GetZone().GetHoles(), 1)
	assert.Len(t, res.GetZone().GetHoles()[0].GetPoints(), 5)
}

func TestCreateZone_RepositoryErrorsAreMapped(t *testing.T) {
	tests := []struct {
		name string
		repo *errRepo
		want codes.Code
	}{
		{"next code failure is internal", &errRepo{nextErr: errors.New("db down")}, codes.Internal},
		{"save failure is internal", &errRepo{saveErr: errors.New("db down")}, codes.Internal},
		{"deadline is mapped", &errRepo{saveErr: context.DeadlineExceeded}, codes.DeadlineExceeded},
		{"cancellation is mapped", &errRepo{saveErr: context.Canceled}, codes.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newErrServer(tt.repo)

			_, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})

			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestUpdateZone_RequiresIdentityAndMapsErrors(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	require.NoError(t, err)
	id := created.GetZone().GetId()

	noActor := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-role", "ADMIN"))
	_, err = srv.UpdateZone(noActor, &zonesv1.UpdateZoneRequest{Id: id, Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = srv.UpdateZone(asRole("ADMIN"), &zonesv1.UpdateZoneRequest{Id: "no-es-uuid", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = srv.UpdateZone(asRole("ADMIN"), &zonesv1.UpdateZoneRequest{Id: id, Name: "@@", Ring: validRing(), Settings: validProtoSettings()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUpdateZone_InactiveZoneIsFailedPrecondition(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	require.NoError(t, err)
	id := created.GetZone().GetId()
	_, err = srv.SetZoneStatus(asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "INACTIVE", Reason: "Obras en la avenida"})
	require.NoError(t, err)

	_, err = srv.UpdateZone(asRole("ADMIN"), &zonesv1.UpdateZoneRequest{Id: id, Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})

	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestSetZoneStatus_MapsValidationAndStateErrors(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	require.NoError(t, err)
	id := created.GetZone().GetId()

	tests := []struct {
		name string
		ctx  context.Context
		req  *zonesv1.SetZoneStatusRequest
		want codes.Code
	}{
		{"unknown status is invalid", asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "PAUSED", Reason: "Motivo valido"}, codes.InvalidArgument},
		{"malformed identifier is not found", asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: "no-es-uuid", Status: "INACTIVE", Reason: "Motivo valido"}, codes.NotFound},
		{"same status is a failed precondition", asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "ACTIVE", Reason: "Motivo valido"}, codes.FailedPrecondition},
		{"operator cannot change status", asRole("OPERATOR"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "INACTIVE", Reason: "Motivo valido"}, codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := srv.SetZoneStatus(tt.ctx, tt.req)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestDeleteZone_InactiveZoneIsDeletedAndMissingIsNotFound(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	require.NoError(t, err)
	id := created.GetZone().GetId()
	_, err = srv.SetZoneStatus(asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "INACTIVE", Reason: "Obras en la avenida"})
	require.NoError(t, err)

	_, err = srv.DeleteZone(asRole("ADMIN"), &zonesv1.DeleteZoneRequest{Id: id, Reason: "Retiro definitivo"})
	require.NoError(t, err)

	_, err = srv.DeleteZone(asRole("ADMIN"), &zonesv1.DeleteZoneRequest{Id: id, Reason: "Retiro definitivo"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestDeleteZone_RequiresIdentityAndReason(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing(), Settings: validProtoSettings()})
	require.NoError(t, err)
	id := created.GetZone().GetId()

	noActor := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-role", "ADMIN"))
	_, err = srv.DeleteZone(noActor, &zonesv1.DeleteZoneRequest{Id: id, Reason: "Retiro definitivo"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = srv.DeleteZone(asRole("ADMIN"), &zonesv1.DeleteZoneRequest{Id: id, Reason: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetZone_ReturnsHolesAndMapsRepositoryErrors(t *testing.T) {
	outer, holes := donutRing()
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "DONA", Name: "Zona Dona", Ring: outer, Holes: holes, Settings: validProtoSettings()})
	require.NoError(t, err)

	res, err := srv.GetZone(asRole("OPERATOR"), &zonesv1.GetZoneRequest{Id: created.GetZone().GetId()})

	require.NoError(t, err)
	assert.Len(t, res.GetZone().GetHoles(), 1)

	_, err = newServer().GetZone(context.Background(), &zonesv1.GetZoneRequest{Id: created.GetZone().GetId()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	failing := newErrServer(&errRepo{findErr: errors.New("db down")})
	_, err = failing.GetZone(asRole("ADMIN"), &zonesv1.GetZoneRequest{Id: created.GetZone().GetId()})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestClassifyPoint_MapsRepositoryErrors(t *testing.T) {
	point := &zonesv1.Coordinate{Latitude: -0.15, Longitude: -78.45}
	tests := []struct {
		name string
		repo *errRepo
		want codes.Code
	}{
		{"database failure is internal", &errRepo{containingErr: errors.New("db down")}, codes.Internal},
		{"deadline is mapped", &errRepo{containingErr: context.DeadlineExceeded}, codes.DeadlineExceeded},
		{"cancellation is mapped", &errRepo{containingErr: context.Canceled}, codes.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newErrServer(tt.repo).ClassifyPoint(asRole("SERVICE"), &zonesv1.ClassifyPointRequest{Point: point})
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}
