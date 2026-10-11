package grpc_test

import (
	"context"
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

// memRepo es un repositorio en memoria para probar la traducción gRPC.
type memRepo struct {
	zones map[string]domain.Zone
}

func (r *memRepo) Save(_ context.Context, z domain.Zone) (domain.Zone, error) {
	for _, existing := range r.zones {
		if existing.Code == z.Code {
			return domain.Zone{}, domain.ErrZoneCodeTaken
		}
	}
	z.ID = "11111111-1111-1111-1111-111111111111"
	r.zones[z.ID] = z
	return z, nil
}

func (r *memRepo) FindByID(_ context.Context, id string) (domain.Zone, error) {
	if z, ok := r.zones[id]; ok {
		return z, nil
	}
	return domain.Zone{}, domain.ErrZoneNotFound
}

func (r *memRepo) FindContaining(_ context.Context, _ domain.Coordinate) (domain.Zone, error) {
	return domain.Zone{}, domain.ErrZoneNotFound
}

func (r *memRepo) Update(_ context.Context, z domain.Zone) (domain.Zone, error) {
	if _, ok := r.zones[z.ID]; !ok {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	r.zones[z.ID] = z
	return z, nil
}

func (r *memRepo) ListAll(_ context.Context) ([]domain.Zone, error) {
	out := make([]domain.Zone, 0, len(r.zones))
	for _, z := range r.zones {
		out = append(out, z)
	}
	return out, nil
}

func (r *memRepo) SetStatus(_ context.Context, id, status, reason string) (domain.Zone, error) {
	z, ok := r.zones[id]
	if !ok {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	z.Status, z.StatusReason = status, reason
	r.zones[id] = z
	return z, nil
}

func (r *memRepo) Delete(_ context.Context, id, _ string) error {
	z, ok := r.zones[id]
	if !ok {
		return domain.ErrZoneNotFound
	}
	if z.Status != domain.StatusInactive {
		return domain.ErrZoneNotInactive
	}
	delete(r.zones, id)
	return nil
}

func (r *memRepo) NextCode(_ context.Context, _ string) (int, error) {
	return 1, nil
}

func newServer() *grpcadapter.Server {
	return grpcadapter.NewServer(application.NewZoneService(&memRepo{zones: map[string]domain.Zone{}}))
}

func TestListZones_AdminAndOperatorOnly(t *testing.T) {
	srv := newServer()
	_, err := srv.ListZones(asRole("DRIVER"), &zonesv1.ListZonesRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	res, err := srv.ListZones(asRole("OPERATOR"), &zonesv1.ListZonesRequest{})
	require.NoError(t, err)
	assert.Empty(t, res.GetZones())
}

func TestUpdateZone_OnlyAdminAndKeepsID(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing()})
	require.NoError(t, err)
	id := created.GetZone().GetId()

	_, err = srv.UpdateZone(asRole("OPERATOR"), &zonesv1.UpdateZoneRequest{Id: id, Code: "NORTE", Name: "X", Ring: validRing()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	res, err := srv.UpdateZone(asRole("ADMIN"), &zonesv1.UpdateZoneRequest{Id: id, Code: "NORTE", Name: "Zona Norte 2", Ring: validRing()})
	require.NoError(t, err)
	assert.Equal(t, id, res.GetZone().GetId())
	assert.Equal(t, "Zona Norte 2", res.GetZone().GetName())
}

func TestUpdateZone_MissingIsNotFound(t *testing.T) {
	_, err := newServer().UpdateZone(asRole("ADMIN"), &zonesv1.UpdateZoneRequest{Id: "11111111-1111-1111-1111-111111111111", Code: "X", Name: "X", Ring: validRing()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func validRing() []*zonesv1.Coordinate {
	return []*zonesv1.Coordinate{
		{Latitude: -0.20, Longitude: -78.50},
		{Latitude: -0.20, Longitude: -78.40},
		{Latitude: -0.10, Longitude: -78.40},
		{Latitude: -0.10, Longitude: -78.50},
		{Latitude: -0.20, Longitude: -78.50},
	}
}

func TestCreateZone_ReturnsZoneWithID(t *testing.T) {
	srv := newServer()

	res, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{
		Code: "NORTE", Name: "Zona Norte", Ring: validRing(),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, res.GetZone().GetId())
	assert.Len(t, res.GetZone().GetRing(), 5)
}

func TestCreateZone_InvalidRingIsInvalidArgument(t *testing.T) {
	srv := newServer()

	_, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{
		Code: "NORTE", Name: "Zona Norte", Ring: validRing()[:4], // anillo abierto y corto
	})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateZone_DuplicateCodeIsAlreadyExists(t *testing.T) {
	srv := newServer()
	req := &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing()}
	_, err := srv.CreateZone(asRole("ADMIN"), req)
	require.NoError(t, err)

	_, err = srv.CreateZone(asRole("ADMIN"), req)

	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestGetZone_MissingIsNotFound(t *testing.T) {
	srv := newServer()

	_, err := srv.GetZone(asRole("ADMIN"), &zonesv1.GetZoneRequest{Id: "11111111-1111-1111-1111-111111111111"})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestClassifyPoint_OutOfRangeIsInvalidArgument(t *testing.T) {
	srv := newServer()

	_, err := srv.ClassifyPoint(asRole("ADMIN"), &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: 200, Longitude: 0},
	})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestClassifyPoint_NoZoneIsNotFound(t *testing.T) {
	srv := newServer()

	_, err := srv.ClassifyPoint(asRole("ADMIN"), &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: -0.15, Longitude: -78.45},
	})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

// asRole simula la identidad que el API Gateway propaga tras validar el JWT.
func asRole(role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-role", role, "x-user-id", "user-test"))
}

func TestClassifyPoint_MissingPointIsInvalidArgument(t *testing.T) {
	_, err := newServer().ClassifyPoint(asRole("ADMIN"), &zonesv1.ClassifyPointRequest{})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestOperationsRequireIdentity(t *testing.T) {
	srv := newServer()
	_, err := srv.GetZone(context.Background(), &zonesv1.GetZoneRequest{Id: "11111111-1111-1111-1111-111111111111"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCreateZone_OnlyAdmin(t *testing.T) {
	for _, role := range []string{"OPERATOR", "DRIVER"} {
		_, err := newServer().CreateZone(asRole(role), &zonesv1.CreateZoneRequest{Code: "X", Name: "X", Ring: validRing()})
		assert.Equal(t, codes.PermissionDenied, status.Code(err), role)
	}
}

func TestReadZone_AdminAndOperatorOnly(t *testing.T) {
	srv := newServer()
	for _, role := range []string{"DRIVER"} {
		_, err := srv.GetZone(asRole(role), &zonesv1.GetZoneRequest{Id: "11111111-1111-1111-1111-111111111111"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err), role)
		_, err = srv.ClassifyPoint(asRole(role), &zonesv1.ClassifyPointRequest{Point: &zonesv1.Coordinate{Latitude: -0.15, Longitude: -78.45}})
		assert.Equal(t, codes.PermissionDenied, status.Code(err), role)
	}
	_, err := srv.ClassifyPoint(asRole("OPERATOR"), &zonesv1.ClassifyPointRequest{Point: &zonesv1.Coordinate{Latitude: -0.15, Longitude: -78.45}})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestSetZoneStatus_RequiresActorAndReason(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing()})
	require.NoError(t, err)
	id := created.GetZone().GetId()

	noActor := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-role", "ADMIN"))
	_, err = srv.SetZoneStatus(noActor, &zonesv1.SetZoneStatusRequest{Id: id, Status: "INACTIVE", Reason: "Obras en la avenida"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "sin usuario no hay bitácora")

	_, err = srv.SetZoneStatus(asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "INACTIVE", Reason: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "el motivo es obligatorio")

	res, err := srv.SetZoneStatus(asRole("ADMIN"), &zonesv1.SetZoneStatusRequest{Id: id, Status: "INACTIVE", Reason: "Obras en la avenida"})
	require.NoError(t, err)
	assert.Equal(t, "INACTIVE", res.GetZone().GetStatus())
	assert.Equal(t, "Obras en la avenida", res.GetZone().GetStatusReason())
}

func TestDeleteZone_ActiveZoneIsFailedPrecondition(t *testing.T) {
	srv := newServer()
	created, err := srv.CreateZone(asRole("ADMIN"), &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing()})
	require.NoError(t, err)

	_, err = srv.DeleteZone(asRole("ADMIN"), &zonesv1.DeleteZoneRequest{Id: created.GetZone().GetId(), Reason: "Motivo de prueba"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
