package grpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
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

func newServer() *grpcadapter.Server {
	return grpcadapter.NewServer(application.NewZoneService(&memRepo{zones: map[string]domain.Zone{}}))
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

	res, err := srv.CreateZone(context.Background(), &zonesv1.CreateZoneRequest{
		Code: "NORTE", Name: "Zona Norte", Ring: validRing(),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, res.GetZone().GetId())
	assert.Len(t, res.GetZone().GetRing(), 5)
}

func TestCreateZone_InvalidRingIsInvalidArgument(t *testing.T) {
	srv := newServer()

	_, err := srv.CreateZone(context.Background(), &zonesv1.CreateZoneRequest{
		Code: "NORTE", Name: "Zona Norte", Ring: validRing()[:4], // anillo abierto y corto
	})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateZone_DuplicateCodeIsAlreadyExists(t *testing.T) {
	srv := newServer()
	req := &zonesv1.CreateZoneRequest{Code: "NORTE", Name: "Zona Norte", Ring: validRing()}
	_, err := srv.CreateZone(context.Background(), req)
	require.NoError(t, err)

	_, err = srv.CreateZone(context.Background(), req)

	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestGetZone_MissingIsNotFound(t *testing.T) {
	srv := newServer()

	_, err := srv.GetZone(context.Background(), &zonesv1.GetZoneRequest{Id: "11111111-1111-1111-1111-111111111111"})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestClassifyPoint_OutOfRangeIsInvalidArgument(t *testing.T) {
	srv := newServer()

	_, err := srv.ClassifyPoint(context.Background(), &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: 200, Longitude: 0},
	})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestClassifyPoint_NoZoneIsNotFound(t *testing.T) {
	srv := newServer()

	_, err := srv.ClassifyPoint(context.Background(), &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: -0.15, Longitude: -78.45},
	})

	assert.Equal(t, codes.NotFound, status.Code(err))
}
