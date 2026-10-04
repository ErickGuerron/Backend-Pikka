package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

// Clave de prueba generada para no dejar literales con forma de secreto.
var secret = strings.Repeat("k", 32)

type fakeAuth struct {
	lastMeta grpcclient.CallMeta
	err      error
}

func (f *fakeAuth) Login(_ context.Context, m grpcclient.CallMeta, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	f.lastMeta = m
	if f.err != nil {
		return nil, f.err
	}
	if req.GetPassword() != "password123" {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	return &authv1.LoginResponse{AccessToken: "tok", TokenType: "Bearer", ExpiresInSeconds: 3600,
		User: &authv1.User{Id: "u-1", Email: req.GetEmail(), Role: authv1.Role_ROLE_ADMIN, Active: true}}, nil
}

func (f *fakeAuth) CreateUser(_ context.Context, m grpcclient.CallMeta, req *authv1.CreateUserRequest) (*authv1.CreateUserResponse, error) {
	f.lastMeta = m
	if f.err != nil {
		return nil, f.err
	}
	return &authv1.CreateUserResponse{User: &authv1.User{Id: "u-2", Email: req.GetEmail(), Role: req.GetRole(), FullName: req.GetFullName(), Active: true}}, nil
}

func (f *fakeAuth) GetUser(_ context.Context, m grpcclient.CallMeta, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error) {
	f.lastMeta = m
	if f.err != nil {
		return nil, f.err
	}
	return &authv1.GetUserResponse{User: &authv1.User{Id: req.GetId(), Role: authv1.Role_ROLE_DRIVER}}, nil
}

func token(t *testing.T, sub, role string) string {
	t.Helper()
	s, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"sub": sub, "role": role, "iss": "lastmile-auth", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

func newRouter(auth httpapi.AuthClient, ready bool) http.Handler {
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		RequestTimeout: time.Second, BodyLimit: "1K", EnableHSTS: true,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), auth, func(context.Context) bool { return ready })
}

func do(h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type errorBody struct {
	Error httpapi.APIError `json:"error"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) httpapi.APIError {
	t.Helper()
	var b errorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &b))
	return b.Error
}

func TestHealthAndReady(t *testing.T) {
	assert.Equal(t, http.StatusOK, do(newRouter(&fakeAuth{}, true), http.MethodGet, "/health", "", nil).Code)
	assert.Equal(t, http.StatusOK, do(newRouter(&fakeAuth{}, true), http.MethodGet, "/ready", "", nil).Code)
	assert.Equal(t, http.StatusServiceUnavailable, do(newRouter(&fakeAuth{}, false), http.MethodGet, "/ready", "", nil).Code)
}

func TestSecurityHeaders(t *testing.T) {
	rec := do(newRouter(&fakeAuth{}, true), http.MethodGet, "/health", "", nil)
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}

func TestRequestIDPropagation(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)

	rec := do(h, http.MethodPost, "/api/v1/auth/login", `{"email":"a@example.com","password":"password123"}`, map[string]string{"X-Request-Id": "abc-123"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "abc-123", rec.Header().Get("X-Request-Id"))
	assert.Equal(t, "abc-123", auth.lastMeta.RequestID)

	rec = do(h, http.MethodPost, "/api/v1/auth/login", `{"email":"a@example.com","password":"password123"}`, map[string]string{"X-Request-Id": "bad\nid"})
	assert.NotEqual(t, "bad\nid", rec.Header().Get("X-Request-Id"))
	assert.Len(t, auth.lastMeta.RequestID, 36, "se genera un UUID cuando el id entrante no es seguro")
}

func TestLogin(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)

	rec := do(h, http.MethodPost, "/api/v1/auth/login", `{"email":"a@example.com","password":"password123"}`, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tok", body["accessToken"])
	assert.Equal(t, "ADMIN", body["user"].(map[string]any)["role"])

	rec = do(h, http.MethodPost, "/api/v1/auth/login", `{"email":"a@example.com","password":"wrong"}`, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	e := decodeError(t, rec)
	assert.Equal(t, "UNAUTHENTICATED", e.Code)
	assert.NotEmpty(t, e.RequestID)

	rec = do(h, http.MethodPost, "/api/v1/auth/login", `{not json`, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`email=a`))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)

	rec = do(h, http.MethodPost, "/api/v1/auth/login", `{"email":"`+strings.Repeat("a", 2000)+`"}`, nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestAuthenticationRequired(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	for name, hdr := range map[string]string{
		"sin cabecera":   "",
		"esquema basic":  "Basic abc",
		"token inválido": "Bearer nope",
	} {
		t.Run(name, func(t *testing.T) {
			headers := map[string]string{}
			if hdr != "" {
				headers["Authorization"] = hdr
			}
			rec := do(h, http.MethodGet, "/api/v1/auth/me", "", headers)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestIdentityIsPropagated(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)
	rec := do(h, http.MethodGet, "/api/v1/auth/me", "", map[string]string{"Authorization": "Bearer " + token(t, "u-7", "DRIVER")})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "u-7", auth.lastMeta.UserID)
	assert.Equal(t, "DRIVER", auth.lastMeta.Role)
	assert.Contains(t, rec.Body.String(), `"id":"u-7"`)
}

func TestCreateUser(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodPost, "/api/v1/users", `{"email":"d@example.com","password":"password123","fullName":"D","role":"DRIVER"}`,
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), `"role":"DRIVER"`)
}

func TestGRPCErrorsAreTranslated(t *testing.T) {
	cases := map[codes.Code]int{
		codes.PermissionDenied: http.StatusForbidden,
		codes.NotFound:         http.StatusNotFound,
		codes.AlreadyExists:    http.StatusConflict,
		codes.InvalidArgument:  http.StatusBadRequest,
		codes.DeadlineExceeded: http.StatusGatewayTimeout,
		codes.Unavailable:      http.StatusServiceUnavailable,
		codes.Internal:         http.StatusInternalServerError,
	}
	for code, want := range cases {
		t.Run(code.String(), func(t *testing.T) {
			h := newRouter(&fakeAuth{err: status.Error(code, "detail")}, true)
			rec := do(h, http.MethodGet, "/api/v1/users/u-3", "", map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
			assert.Equal(t, want, rec.Code)
			if want == http.StatusInternalServerError {
				assert.Equal(t, "internal error", decodeError(t, rec).Message, "no se filtran detalles internos")
			}
		})
	}
}

func TestCORS(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))

	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestLoginRateLimit(t *testing.T) {
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 2,
		RequestTimeout: time.Second, BodyLimit: "1K",
	}
	h := httpapi.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, func(context.Context) bool { return true })
	body := `{"email":"a@example.com","password":"password123"}`
	assert.Equal(t, http.StatusOK, do(h, http.MethodPost, "/api/v1/auth/login", body, nil).Code)
	rec := do(h, http.MethodPost, "/api/v1/auth/login", body, nil)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "RATE_LIMITED", decodeError(t, rec).Code)
}
