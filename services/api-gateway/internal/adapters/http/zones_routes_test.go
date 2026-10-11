package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

type fakeZones struct {
	lastMeta  grpcclient.CallMeta
	lastRing  int
	lastHoles []*zonesv1.Ring
	lastPt    *zonesv1.Coordinate
	called    bool
	err       error
}

func (f *fakeZones) CreateZone(_ context.Context, m grpcclient.CallMeta, req *zonesv1.CreateZoneRequest) (*zonesv1.CreateZoneResponse, error) {
	f.lastMeta, f.called, f.lastRing, f.lastHoles = m, true, len(req.GetRing()), req.GetHoles()
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.CreateZoneResponse{Zone: &zonesv1.Zone{Id: "z-1", Code: req.GetCode(), Name: req.GetName(), Ring: req.GetRing(), Holes: req.GetHoles()}}, nil
}

func (f *fakeZones) GetZone(_ context.Context, m grpcclient.CallMeta, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	f.lastMeta, f.called = m, true
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.GetZoneResponse{Zone: &zonesv1.Zone{Id: req.GetId(), Code: "CENTRO"}}, nil
}

func (f *fakeZones) ClassifyPoint(_ context.Context, m grpcclient.CallMeta, req *zonesv1.ClassifyPointRequest) (*zonesv1.ClassifyPointResponse, error) {
	f.lastMeta, f.called, f.lastPt = m, true, req.GetPoint()
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.ClassifyPointResponse{Zone: &zonesv1.Zone{Id: "z-1", Code: "CENTRO"}}, nil
}

func newZonesRouter(zones httpapi.ZonesClient) http.Handler {
	opts := httpapi.Options{
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		RequestTimeout: time.Second, BodyLimit: "4K",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, zones, func(context.Context) bool { return true })
}

func bearer(t *testing.T, role string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token(t, "u-1", role)}
}

const validZoneJSON = `{"code":"CENTRO","name":"Centro","ring":[
	{"latitude":-0.2,"longitude":-78.5},{"latitude":-0.2,"longitude":-78.4},
	{"latitude":-0.1,"longitude":-78.4},{"latitude":-0.2,"longitude":-78.5}]}`

func TestZones_RequireAuthentication(t *testing.T) {
	h := newZonesRouter(&fakeZones{})
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/api/v1/zones/z-1", "", nil).Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, "/api/v1/zones", validZoneJSON, nil).Code)
}

func TestZones_CreateForwardsIdentityAndBody(t *testing.T) {
	zones := &fakeZones{}
	rec := do(newZonesRouter(zones), http.MethodPost, "/api/v1/zones", validZoneJSON, bearer(t, "ADMIN"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "ADMIN", zones.lastMeta.Role)
	assert.Equal(t, "u-1", zones.lastMeta.UserID)
	assert.Equal(t, 4, zones.lastRing)
}

func TestZones_CreateDeniedRoleReturns403(t *testing.T) {
	zones := &fakeZones{err: status.Error(codes.PermissionDenied, "forbidden")}
	rec := do(newZonesRouter(zones), http.MethodPost, "/api/v1/zones", validZoneJSON, bearer(t, "OPERATOR"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code)
}

func TestZones_ClassifyPassesCoordinates(t *testing.T) {
	zones := &fakeZones{}
	rec := do(newZonesRouter(zones), http.MethodGet, "/api/v1/zones/classify?latitude=-0.15&longitude=-78.45", "", bearer(t, "OPERATOR"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, zones.lastPt)
	assert.InDelta(t, -0.15, zones.lastPt.GetLatitude(), 1e-9)
	assert.InDelta(t, -78.45, zones.lastPt.GetLongitude(), 1e-9)
}

func TestZones_ClassifyRejectsNonNumericCoordinates(t *testing.T) {
	zones := &fakeZones{}
	rec := do(newZonesRouter(zones), http.MethodGet, "/api/v1/zones/classify?latitude=abc&longitude=-78", "", bearer(t, "ADMIN"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, zones.called, "no debe llamar al servicio con datos inválidos")
}

func TestZones_ClassifyNotFoundIsReturnedAs404(t *testing.T) {
	zones := &fakeZones{err: status.Error(codes.NotFound, "zone not found")}
	rec := do(newZonesRouter(zones), http.MethodGet, "/api/v1/zones/classify?latitude=0&longitude=0", "", bearer(t, "ADMIN"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "NOT_FOUND", decodeError(t, rec).Code)
}

func TestZones_GetByID(t *testing.T) {
	zones := &fakeZones{}
	rec := do(newZonesRouter(zones), http.MethodGet, "/api/v1/zones/z-9", "", bearer(t, "ADMIN"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"id":"z-9"`)
}

const donutZoneJSON = `{"code":"DONA","name":"Dona","ring":[{"latitude":0,"longitude":0},{"latitude":0,"longitude":4},{"latitude":4,"longitude":4},{"latitude":4,"longitude":0},{"latitude":0,"longitude":0}],"holes":[[{"latitude":1,"longitude":1},{"latitude":1,"longitude":3},{"latitude":3,"longitude":3},{"latitude":3,"longitude":1},{"latitude":1,"longitude":1}]]}`

func TestZones_CreateForwardsHolesAndReturnsThem(t *testing.T) {
	zones := &fakeZones{}
	rec := do(newZonesRouter(zones), http.MethodPost, "/api/v1/zones", donutZoneJSON, bearer(t, "ADMIN"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, zones.lastHoles, 1)
	assert.Len(t, zones.lastHoles[0].GetPoints(), 5)
	assert.Contains(t, rec.Body.String(), `"holes":[[`)
}
