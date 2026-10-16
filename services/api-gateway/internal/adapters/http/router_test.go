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
	lastMeta       grpcclient.CallMeta
	lastUpdate     *authv1.UpdateUserRequest
	lastDeactivate *authv1.DeactivateUserRequest
	err            error
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

func (f *fakeAuth) SignupCustomer(_ context.Context, m grpcclient.CallMeta, req *authv1.SignupCustomerRequest) (*authv1.SignupCustomerResponse, error) {
	f.lastMeta = m
	if f.err != nil {
		return nil, f.err
	}
	if len(req.GetPassword()) < 8 {
		return nil, status.Error(codes.InvalidArgument, "password must be between 8 and 72 bytes")
	}
	return &authv1.SignupCustomerResponse{AccessToken: "tok-customer", TokenType: "Bearer", ExpiresInSeconds: 3600,
		User: &authv1.User{Id: "u-customer", Email: req.GetEmail(), FullName: req.GetFullName(), Role: authv1.Role_ROLE_CUSTOMER, Active: true, CreatedAt: "2026-01-02T03:04:05Z"}}, nil
}

// ListUsers devuelve una lista con dos usuarios. Los handlers reciben
// CallMeta con el rol del llamador; el handler debe rechazar (403) si el rol
// no es ADMIN antes de invocar este método. El fake no hace ninguna validación
// de rol porque la lógica vive en el handler HTTP (adminOnlyMiddleware) y en
// el caso de uso (ListUsers del Auth Service).
func (f *fakeAuth) ListUsers(_ context.Context, m grpcclient.CallMeta, _ *authv1.ListUsersRequest) (*authv1.ListUsersResponse, error) {
	f.lastMeta = m
	if f.err != nil {
		return nil, f.err
	}
	return &authv1.ListUsersResponse{
		Users: []*authv1.User{
			{Id: "u-admin", Email: "admin@example.com", FullName: "Admin", Role: authv1.Role_ROLE_ADMIN, Active: true, CreatedAt: "2026-01-01T00:00:00Z"},
			{Id: "u-customer", Email: "cliente@example.com", FullName: "Cliente", Role: authv1.Role_ROLE_CUSTOMER, Active: true, CreatedAt: "2026-01-02T00:00:00Z"},
		},
	}, nil
}

// UpdateUser refleja el reenvío: guarda la petición para que el test compruebe
// qué campos llegaron. Rol UNSPECIFIED (CUSTOMER o inválido) se rechaza como
// lo hace auth-service.
func (f *fakeAuth) UpdateUser(_ context.Context, m grpcclient.CallMeta, req *authv1.UpdateUserRequest) (*authv1.UpdateUserResponse, error) {
	f.lastMeta = m
	f.lastUpdate = req
	if f.err != nil {
		return nil, f.err
	}
	if req.Role != nil && req.GetRole() == authv1.Role_ROLE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "invalid role")
	}
	return &authv1.UpdateUserResponse{User: &authv1.User{Id: req.GetId(), FullName: req.GetFullName(), Role: authv1.Role_ROLE_DRIVER, Active: true}}, nil
}

func (f *fakeAuth) DeactivateUser(_ context.Context, m grpcclient.CallMeta, req *authv1.DeactivateUserRequest) (*authv1.DeactivateUserResponse, error) {
	f.lastMeta = m
	f.lastDeactivate = req
	if f.err != nil {
		return nil, f.err
	}
	return &authv1.DeactivateUserResponse{User: &authv1.User{Id: req.GetId(), Role: authv1.Role_ROLE_DRIVER, Active: false}}, nil
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
	return httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), auth, nil, func(context.Context) bool { return ready })
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

// --- BE-T3: GET /api/v1/users (solo ADMIN) ---------------------------------

func TestListUsers_AsAdmin_Returns200(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)

	rec := do(h, http.MethodGet, "/api/v1/users", "",
		map[string]string{"Authorization": "Bearer " + token(t, "u-1", "ADMIN")})
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	users, ok := body["users"].([]any)
	require.True(t, ok, "la respuesta debe tener clave users como array")
	assert.Len(t, users, 2)

	// El rol del caller se propagó al backend vía CallMeta.
	assert.Equal(t, "u-1", auth.lastMeta.UserID)
	assert.Equal(t, "ADMIN", auth.lastMeta.Role)
}

func TestListUsers_AsCustomer_Returns403(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)

	rec := do(h, http.MethodGet, "/api/v1/users", "",
		map[string]string{"Authorization": "Bearer " + token(t, "u-9", "CUSTOMER")})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code)

	// El middleware corta antes de llegar al handler, así que CallMeta queda vacío.
	assert.Equal(t, "", auth.lastMeta.UserID)
}

func TestListUsers_AsOperator_Returns403(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)

	rec := do(h, http.MethodGet, "/api/v1/users", "",
		map[string]string{"Authorization": "Bearer " + token(t, "u-3", "OPERATOR")})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code)
}

func TestListUsers_AsDriver_Returns403(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)

	rec := do(h, http.MethodGet, "/api/v1/users", "",
		map[string]string{"Authorization": "Bearer " + token(t, "u-7", "DRIVER")})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code)
}

func TestListUsers_WithoutAuth_Returns401(t *testing.T) {
	auth := &fakeAuth{}
	h := newRouter(auth, true)

	rec := do(h, http.MethodGet, "/api/v1/users", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "UNAUTHENTICATED", decodeError(t, rec).Code)
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
	h := httpapi.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, func(context.Context) bool { return true })
	body := `{"email":"a@example.com","password":"password123"}`
	assert.Equal(t, http.StatusOK, do(h, http.MethodPost, "/api/v1/auth/login", body, nil).Code)
	rec := do(h, http.MethodPost, "/api/v1/auth/login", body, nil)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "RATE_LIMITED", decodeError(t, rec).Code)
}

func TestAPIDocs(t *testing.T) {
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		RequestTimeout: time.Second, BodyLimit: "1K", EnableDocs: true,
	}
	h := httpapi.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, func(context.Context) bool { return true })

	rec := do(h, http.MethodGet, "/docs", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `data-url="/openapi.yaml"`)
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "https://cdn.jsdelivr.net")

	rec = do(h, http.MethodGet, "/openapi.yaml", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "openapi: 3.0.3")
	assert.Contains(t, rec.Header().Get("Content-Type"), "yaml")

	// La CSP estricta sigue aplicando al resto de rutas.
	rec = do(h, http.MethodGet, "/health", "", nil)
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
}

func TestAPIDocsDisabled(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, "/docs", "", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, "/openapi.yaml", "", nil).Code)
}
