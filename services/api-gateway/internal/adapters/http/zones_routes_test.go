package http_test

import (
	"context"
	"encoding/json"
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

func (f *fakeZones) ListZones(_ context.Context, m grpcclient.CallMeta, _ *zonesv1.ListZonesRequest) (*zonesv1.ListZonesResponse, error) {
	f.lastMeta, f.called = m, true
	if f.err != nil {
		return nil, f.err
	}
	// Zone Service reserva el listado completo al personal; el Gateway no decide por rol.
	if m.Role == "CUSTOMER" {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}
	return &zonesv1.ListZonesResponse{Zones: []*zonesv1.Zone{{Id: "z-1", Code: "CENTRO", Name: "Centro"}}}, nil
}

func (f *fakeZones) ListCoverageZones(_ context.Context, m grpcclient.CallMeta, _ *zonesv1.ListCoverageZonesRequest) (*zonesv1.ListCoverageZonesResponse, error) {
	f.lastMeta, f.called = m, true
	if f.err != nil {
		return nil, f.err
	}
	// Zone Service ya devuelve solo zonas activas; aquí llega con datos operativos para comprobar que no se filtran.
	return &zonesv1.ListCoverageZonesResponse{Zones: []*zonesv1.Zone{{
		Id: "z-1", Code: "CENTRO", Name: "Centro", Status: "ACTIVE", StatusReason: "interno", CreatedBy: "admin-1", Version: 3,
		Ring:     []*zonesv1.Coordinate{{Latitude: -0.2, Longitude: -78.5}, {Latitude: -0.2, Longitude: -78.4}, {Latitude: -0.1, Longitude: -78.4}, {Latitude: -0.2, Longitude: -78.5}},
		Holes:    []*zonesv1.Ring{{Points: []*zonesv1.Coordinate{{Latitude: -0.17, Longitude: -78.45}}}},
		Settings: &zonesv1.ZoneSettings{DepotName: "Hub Norte", DefaultRouteCapacityKg: 45},
	}}}, nil
}

func (f *fakeZones) UpdateZone(_ context.Context, m grpcclient.CallMeta, req *zonesv1.UpdateZoneRequest) (*zonesv1.UpdateZoneResponse, error) {
	f.lastMeta, f.called, f.lastRing, f.lastHoles = m, true, len(req.GetRing()), req.GetHoles()
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.UpdateZoneResponse{Zone: &zonesv1.Zone{Id: req.GetId(), Code: req.GetCode(), Name: req.GetName(), Ring: req.GetRing(), Holes: req.GetHoles()}}, nil
}

func TestZones_ListReturnsArray(t *testing.T) {
	rec := do(newZonesRouter(&fakeZones{}), http.MethodGet, "/api/v1/zones", "", bearer(t, "OPERATOR"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"CENTRO"`)
}

func TestZones_UpdateUsesPathIDAndForwardsHoles(t *testing.T) {
	zones := &fakeZones{}
	rec := do(newZonesRouter(zones), http.MethodPut, "/api/v1/zones/z-9", donutZoneJSON, bearer(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"id":"z-9"`)
	require.Len(t, zones.lastHoles, 1)
}

func TestZones_ListAndUpdateRequireAuthentication(t *testing.T) {
	h := newZonesRouter(&fakeZones{})
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/api/v1/zones", "", nil).Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPut, "/api/v1/zones/z-9", donutZoneJSON, nil).Code)
}

func (f *fakeZones) SetZoneStatus(_ context.Context, m grpcclient.CallMeta, req *zonesv1.SetZoneStatusRequest) (*zonesv1.SetZoneStatusResponse, error) {
	f.lastMeta, f.called = m, true
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.SetZoneStatusResponse{Zone: &zonesv1.Zone{Id: req.GetId(), Code: "NORTE-001", Status: req.GetStatus(), StatusReason: req.GetReason()}}, nil
}

func (f *fakeZones) DeleteZone(_ context.Context, m grpcclient.CallMeta, _ *zonesv1.DeleteZoneRequest) (*zonesv1.DeleteZoneResponse, error) {
	f.lastMeta, f.called = m, true
	if f.err != nil {
		return nil, f.err
	}
	return &zonesv1.DeleteZoneResponse{}, nil
}

func TestZones_SetStatusReturnsStatusAndReason(t *testing.T) {
	rec := do(newZonesRouter(&fakeZones{}), http.MethodPatch, "/api/v1/zones/z-9/status", `{"status":"INACTIVE","reason":"Obras en la avenida"}`, bearer(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"INACTIVE"`)
	assert.Contains(t, rec.Body.String(), `"statusReason":"Obras en la avenida"`)
}

func TestZones_DeleteReturnsNoContent(t *testing.T) {
	rec := do(newZonesRouter(&fakeZones{}), http.MethodDelete, "/api/v1/zones/z-9", `{"reason":"Zona creada por error"}`, bearer(t, "ADMIN"))
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

func TestZones_StatusAndDeleteRequireAuthentication(t *testing.T) {
	h := newZonesRouter(&fakeZones{})
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPatch, "/api/v1/zones/z-9/status", `{"status":"INACTIVE","reason":"x"}`, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodDelete, "/api/v1/zones/z-9", `{"reason":"x"}`, nil).Code)
}

func TestCoverage_RequiresAuthentication(t *testing.T) {
	zones := &fakeZones{}
	assert.Equal(t, http.StatusUnauthorized, do(newZonesRouter(zones), http.MethodGet, "/api/v1/coverage/zones", "", nil).Code)
	assert.False(t, zones.called)
}

func TestCoverage_EveryRoleGetsTheSameTrimmedZones(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR", "DRIVER", "CUSTOMER"} {
		t.Run(role, func(t *testing.T) {
			zones := &fakeZones{}
			rec := do(newZonesRouter(zones), http.MethodGet, "/api/v1/coverage/zones", "", bearer(t, role))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, role, zones.lastMeta.Role)

			var body struct {
				Zones []map[string]any `json:"zones"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Zones, 1)
			keys := make([]string, 0, len(body.Zones[0]))
			for k := range body.Zones[0] {
				keys = append(keys, k)
			}
			assert.ElementsMatch(t, []string{"id", "code", "name", "ring", "holes"}, keys)
			assert.Equal(t, "z-1", body.Zones[0]["id"])
			assert.Len(t, body.Zones[0]["ring"], 4)
			assert.Len(t, body.Zones[0]["holes"], 1)
			assert.NotContains(t, rec.Body.String(), "Hub Norte")
		})
	}
}

// noCoverage simula un Zone Service sin zonas activas.
type noCoverage struct{ fakeZones }

func (noCoverage) ListCoverageZones(context.Context, grpcclient.CallMeta, *zonesv1.ListCoverageZonesRequest) (*zonesv1.ListCoverageZonesResponse, error) {
	return &zonesv1.ListCoverageZonesResponse{}, nil
}

func TestCoverage_NoActiveZonesIsAnEmptyArray(t *testing.T) {
	rec := do(newZonesRouter(&noCoverage{}), http.MethodGet, "/api/v1/coverage/zones", "", bearer(t, "CUSTOMER"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"zones":[]}`, rec.Body.String())
}

func TestCoverage_CustomerStillForbiddenOnFullZoneList(t *testing.T) {
	h := newZonesRouter(&fakeZones{})
	assert.Equal(t, http.StatusOK, do(h, http.MethodGet, "/api/v1/coverage/zones", "", bearer(t, "CUSTOMER")).Code)
	rec := do(h, http.MethodGet, "/api/v1/zones", "", bearer(t, "CUSTOMER"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code)
}
