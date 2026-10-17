package http_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

const (
	locationPath      = "/api/v1/drivers/me/location"
	validLocationBody = `{"latitude":-0.2201234,"longitude":-78.5123456,"recordedAt":"2026-10-16T12:00:00Z"}`
	// Coordenadas de la prueba de logs: si aparecen en la salida, el handler o el logger las filtró.
	loggedLatitude  = "0.2201234"
	loggedLongitude = "78.5123456"
)

var errUnexpectedDriverCall = status.Error(codes.Internal, "unexpected call to driver service")

// fakeDriverLookup resuelve el user id autenticado a un repartidor y cuenta las llamadas.
type fakeDriverLookup struct {
	calls      int
	lastUserID string
	err        error
}

func (f *fakeDriverLookup) GetDriverByUserID(_ context.Context, _ grpcclient.CallMeta, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	f.calls++
	f.lastUserID = req.GetUserId()
	if f.err != nil {
		return nil, f.err
	}
	return &driversv1.GetDriverByUserIDResponse{Driver: &driversv1.DriverSummary{Id: "d-1", UserId: req.GetUserId()}}, nil
}

func (f *fakeDriverLookup) RegisterDelivery(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.RegisterDeliveryRequest) (*driversv1.RegisterDeliveryResponse, error) {
	return nil, errUnexpectedDriverCall
}

func (f *fakeDriverLookup) CreateDriver(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.CreateDriverRequest) (*driversv1.CreateDriverResponse, error) {
	return nil, errUnexpectedDriverCall
}

func (f *fakeDriverLookup) ListDrivers(_ context.Context, _ grpcclient.CallMeta, _ *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	return nil, errUnexpectedDriverCall
}

// fakeTrackingClient guarda la última posición recibida y cuenta las llamadas.
type fakeTrackingClient struct {
	calls int
	last  *trackingv1.RecordPositionRequest
	err   error
}

func (f *fakeTrackingClient) RecordPosition(_ context.Context, _ grpcclient.CallMeta, req *trackingv1.RecordPositionRequest) (*trackingv1.RecordPositionResponse, error) {
	f.calls++
	f.last = req
	if f.err != nil {
		return nil, f.err
	}
	return &trackingv1.RecordPositionResponse{Accepted: true}, nil
}

// locationRouter arma el router con los clientes falsos. Los logs van a out para poder inspeccionarlos.
func locationRouter(drivers httpapi.DriverClient, tracking httpapi.TrackingClient, out io.Writer) http.Handler {
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		RequestTimeout: time.Second, BodyLimit: "1K", EnableHSTS: true,
		Drivers: drivers, Tracking: tracking,
	}
	log := slog.New(slog.NewJSONHandler(out, nil))
	return httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, func(context.Context) bool { return true })
}

// driverAuth devuelve las cabeceras de un repartidor autenticado (user id "u-driver").
func driverAuth(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": "Bearer " + token(t, "u-driver", "DRIVER")}
}

func TestReportLocation_WithoutToken_Returns401(t *testing.T) {
	drivers, tracking := &fakeDriverLookup{}, &fakeTrackingClient{}
	h := locationRouter(drivers, tracking, io.Discard)

	rec := do(h, http.MethodPut, locationPath, validLocationBody, nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, drivers.calls)
	assert.Zero(t, tracking.calls)
}

func TestReportLocation_NonDriverRole_Returns403BeforeAnyCall(t *testing.T) {
	drivers, tracking := &fakeDriverLookup{}, &fakeTrackingClient{}
	h := locationRouter(drivers, tracking, io.Discard)
	for _, role := range []string{"ADMIN", "OPERATOR", "CUSTOMER"} {
		headers := map[string]string{"Authorization": "Bearer " + token(t, "u-"+role, role)}
		rec := do(h, http.MethodPut, locationPath, validLocationBody, headers)
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
	}
	assert.Zero(t, drivers.calls)
	assert.Zero(t, tracking.calls)
}

func TestReportLocation_DriverValidPosition_Returns204AndForwardsToTracking(t *testing.T) {
	drivers, tracking := &fakeDriverLookup{}, &fakeTrackingClient{}
	h := locationRouter(drivers, tracking, io.Discard)

	rec := do(h, http.MethodPut, locationPath, validLocationBody, driverAuth(t))

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "u-driver", drivers.lastUserID)
	require.Equal(t, 1, tracking.calls)
	require.NotNil(t, tracking.last)
	assert.Equal(t, "d-1", tracking.last.GetDriverId())
	assert.InDelta(t, -0.2201234, tracking.last.GetLatitude(), 1e-9)
	assert.InDelta(t, -78.5123456, tracking.last.GetLongitude(), 1e-9)
	assert.Equal(t, "2026-10-16T12:00:00Z", tracking.last.GetRecordedAt())
}

// La forma del cuerpo se valida en el gateway: ausencia, tipo y formato de fecha. Sin
// llamadas a los servicios internos cuando el cuerpo es inválido.
func TestReportLocation_InvalidShape_Returns400WithoutInternalCalls(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"latitude":`},
		{"latitude missing", `{"longitude":-78.5,"recordedAt":"2026-10-16T12:00:00Z"}`},
		{"longitude missing", `{"latitude":-0.2,"recordedAt":"2026-10-16T12:00:00Z"}`},
		{"recordedAt missing", `{"latitude":-0.2,"longitude":-78.5}`},
		{"recordedAt not RFC 3339", `{"latitude":-0.2,"longitude":-78.5,"recordedAt":"16/10/2026 12:00"}`},
		{"latitude is a string", `{"latitude":"-0.2","longitude":-78.5,"recordedAt":"2026-10-16T12:00:00Z"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			drivers, tracking := &fakeDriverLookup{}, &fakeTrackingClient{}
			h := locationRouter(drivers, tracking, io.Discard)

			rec := do(h, http.MethodPut, locationPath, tc.body, driverAuth(t))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "INVALID_ARGUMENT", decodeError(t, rec).Code)
			assert.Zero(t, drivers.calls)
			assert.Zero(t, tracking.calls)
		})
	}
}

func TestReportLocation_WrongContentType_Returns415(t *testing.T) {
	drivers, tracking := &fakeDriverLookup{}, &fakeTrackingClient{}
	h := locationRouter(drivers, tracking, io.Discard)
	req := httptest.NewRequest(http.MethodPut, locationPath, strings.NewReader(validLocationBody))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", driverAuth(t)["Authorization"])
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.Zero(t, tracking.calls)
}

// Los errores de Tracking se traducen con el mismo mapa que el resto de la API. Ningún caso
// responde 204 si el servicio rechazó la posición.
func TestReportLocation_TrackingErrors_MapToErrorEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantName string
	}{
		{"out of range rejected by tracking", status.Error(codes.InvalidArgument, "driver position is invalid"), http.StatusBadRequest, "INVALID_ARGUMENT"},
		{"future timestamp rejected by tracking", status.Error(codes.InvalidArgument, "driver position timestamp is ahead of the server clock"), http.StatusBadRequest, "INVALID_ARGUMENT"},
		{"tracking unavailable", status.Error(codes.Unavailable, "tracking unavailable"), http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE"},
		{"tracking internal failure", status.Error(codes.Internal, "internal error"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			drivers := &fakeDriverLookup{}
			tracking := &fakeTrackingClient{err: tc.err}
			h := locationRouter(drivers, tracking, io.Discard)

			rec := do(h, http.MethodPut, locationPath, validLocationBody, driverAuth(t))

			assert.Equal(t, tc.wantCode, rec.Code)
			assert.Equal(t, tc.wantName, decodeError(t, rec).Code)
			assert.Equal(t, 1, tracking.calls)
		})
	}
}

func TestReportLocation_UnknownDriver_Returns404AndNeverCallsTracking(t *testing.T) {
	drivers := &fakeDriverLookup{err: status.Error(codes.NotFound, "driver not found")}
	tracking := &fakeTrackingClient{}
	h := locationRouter(drivers, tracking, io.Discard)

	rec := do(h, http.MethodPut, locationPath, validLocationBody, driverAuth(t))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "NOT_FOUND", decodeError(t, rec).Code)
	assert.Zero(t, tracking.calls)
}

// Las coordenadas nunca deben aparecer en los logs. El logger es inyectable, así que el test
// observa la salida real del middleware y del handler en el camino de éxito y en el de error.
func TestReportLocation_DoesNotLogCoordinates(t *testing.T) {
	var logs bytes.Buffer
	drivers := &fakeDriverLookup{}
	tracking := &fakeTrackingClient{}
	h := locationRouter(drivers, tracking, &logs)

	okRec := do(h, http.MethodPut, locationPath, validLocationBody, driverAuth(t))
	require.Equal(t, http.StatusNoContent, okRec.Code)

	tracking.err = status.Error(codes.Unavailable, "tracking unavailable")
	errRec := do(h, http.MethodPut, locationPath, validLocationBody, driverAuth(t))
	require.Equal(t, http.StatusServiceUnavailable, errRec.Code)

	out := logs.String()
	require.Contains(t, out, `"http request"`, "the request logger must have run, so the assertion means something")
	assert.NotContains(t, out, loggedLatitude)
	assert.NotContains(t, out, loggedLongitude)
}
