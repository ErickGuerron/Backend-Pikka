package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

// signupFakeAuth extiende fakeAuth con un override de SignupCustomer que
// distingue entre los escenarios del test (correo inválido, duplicado, válido).
// Hereda el resto de los métodos del fakeAuth definido en router_test.go.
type signupFakeAuth struct {
	*fakeAuth
}

func (s *signupFakeAuth) SignupCustomer(_ context.Context, m grpcclient.CallMeta, req *authv1.SignupCustomerRequest) (*authv1.SignupCustomerResponse, error) {
	s.lastMeta = m
	if s.err != nil {
		return nil, s.err
	}
	if req.GetEmail() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid email")
	}
	if len(req.GetPassword()) < 8 {
		return nil, status.Error(codes.InvalidArgument, "password must be between 8 and 72 bytes")
	}
	if strings.EqualFold(req.GetEmail(), "duplicado@example.com") {
		return nil, status.Error(codes.AlreadyExists, "email already registered")
	}
	return &authv1.SignupCustomerResponse{
		AccessToken:      "tok-customer",
		TokenType:        "Bearer",
		ExpiresInSeconds: 3600,
		User: &authv1.User{
			Id:        "u-customer",
			Email:     req.GetEmail(),
			FullName:  req.GetFullName(),
			Role:      authv1.Role_ROLE_CUSTOMER,
			Active:    true,
			CreatedAt: "2026-01-02T03:04:05Z",
		},
	}, nil
}

func newRouterWithSignup(auth httpapi.AuthClient, opts httpapi.Options) http.Handler {
	if opts.CORSAllowedOrigins == nil {
		opts.CORSAllowedOrigins = []string{"http://localhost:5173"}
	}
	if opts.RateLimitPerSecond == 0 {
		opts.RateLimitPerSecond = 1000
		opts.RateLimitBurst = 1000
		opts.LoginRatePerMinute = 6000
		opts.SignupRatePerMinute = 6000
		opts.RequestTimeout = time.Second
		opts.BodyLimit = "1K"
		opts.EnableHSTS = true
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), auth, nil, func(context.Context) bool { return true })
}

func TestSignup_Valid_Returns201WithToken(t *testing.T) {
	h := newRouterWithSignup(&signupFakeAuth{fakeAuth: &fakeAuth{}}, httpapi.Options{})

	body := `{"email":"cliente@example.com","password":"password123","fullName":"Cliente Nuevo"}`
	rec := do(h, http.MethodPost, "/api/v1/auth/signup", body, nil)
	require.Equal(t, http.StatusCreated, rec.Code, "signup válido devuelve 201")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "tok-customer", resp["accessToken"])
	assert.Equal(t, "Bearer", resp["tokenType"])
	assert.EqualValues(t, 3600, resp["expiresIn"])
	user := resp["user"].(map[string]any)
	assert.Equal(t, "cliente@example.com", user["email"])
	assert.Equal(t, "CUSTOMER", user["role"])
}

func TestSignup_DuplicateEmail_Returns409(t *testing.T) {
	h := newRouterWithSignup(&signupFakeAuth{fakeAuth: &fakeAuth{}}, httpapi.Options{})

	body := `{"email":"duplicado@example.com","password":"password123","fullName":"X"}`
	rec := do(h, http.MethodPost, "/api/v1/auth/signup", body, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "CONFLICT", decodeError(t, rec).Code)
}

func TestSignup_RateLimited_Returns429(t *testing.T) {
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		SignupRatePerMinute: 3,
		RequestTimeout:      time.Second, BodyLimit: "1K", EnableHSTS: true,
	}
	h := newRouterWithSignup(&signupFakeAuth{fakeAuth: &fakeAuth{}}, opts)

	// Con SignupRatePerMinute=3 el bucket dedicado tiene burst=1 (max(1, int(3/2))).
	// La primera llamada consume el token inicial; la segunda llega antes de que
	// se reponga (rate = 3/min ≈ 20 s/token) y debe devolver 429.
	assert.Equal(t, http.StatusCreated, do(h, http.MethodPost, "/api/v1/auth/signup",
		`{"email":"a@example.com","password":"password123","fullName":"A"}`, nil).Code)
	rec := do(h, http.MethodPost, "/api/v1/auth/signup",
		`{"email":"b@example.com","password":"password123","fullName":"B"}`, nil)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "RATE_LIMITED", decodeError(t, rec).Code)
}
