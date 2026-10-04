package grpc_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/security"
)

type memRepo struct {
	mu    sync.Mutex
	users []domain.User
}

func (r *memRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.users {
		if e.Email == u.Email {
			return domain.User{}, domain.ErrEmailAlreadyExists
		}
	}
	u.ID = fmt.Sprintf("u-%d", len(r.users)+1)
	u.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r.users = append(r.users, u)
	return u, nil
}

func (r *memRepo) find(match func(domain.User) bool) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if match(u) {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (r *memRepo) FindByEmail(_ context.Context, email string) (domain.User, error) {
	return r.find(func(u domain.User) bool { return u.Email == email })
}

func (r *memRepo) FindByID(_ context.Context, id string) (domain.User, error) {
	return r.find(func(u domain.User) bool { return u.ID == id })
}

func (r *memRepo) Count(context.Context) (int, error) { return len(r.users), nil }

func newClient(t *testing.T) authv1.AuthServiceClient {
	t.Helper()
	repo := &memRepo{}
	hasher := security.NewBcryptHasher(bcrypt.MinCost)
	tokens := security.NewJWTIssuer(strings.Repeat("k", 32), "lastmile-auth", time.Hour)
	login, err := application.NewLogin(repo, hasher, tokens)
	require.NoError(t, err)
	_, err = application.NewBootstrapAdmin(repo, hasher).Execute(context.Background(), "admin@example.com", "admin-password")
	require.NoError(t, err)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpcadapter.UnaryInterceptors(slog.New(slog.NewTextHandler(io.Discard, nil))))
	authv1.RegisterAuthServiceServer(srv, grpcadapter.NewServer(grpcadapter.UseCases{
		Login:         login,
		CreateUser:    application.NewCreateUser(repo, hasher),
		GetUser:       application.NewGetUser(repo),
		ValidateToken: application.NewValidateToken(tokens),
	}))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return authv1.NewAuthServiceClient(conn)
}

func asCaller(id, role string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), grpcadapter.MetadataUserID, id, grpcadapter.MetadataUserRole, role)
}

func TestLoginAndValidate(t *testing.T) {
	c := newClient(t)

	res, err := c.Login(context.Background(), &authv1.LoginRequest{Email: "admin@example.com", Password: "admin-password"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer", res.GetTokenType())
	assert.Equal(t, int64(3600), res.GetExpiresInSeconds())
	assert.Equal(t, authv1.Role_ROLE_ADMIN, res.GetUser().GetRole())
	assert.Equal(t, "2026-01-02T03:04:05Z", res.GetUser().GetCreatedAt())

	v, err := c.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{AccessToken: res.GetAccessToken()})
	require.NoError(t, err)
	assert.Equal(t, res.GetUser().GetId(), v.GetUserId())

	_, err = c.Login(context.Background(), &authv1.LoginRequest{Email: "admin@example.com", Password: "nope-nope"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = c.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{AccessToken: "garbage"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCreateUserAuthorization(t *testing.T) {
	c := newClient(t)
	req := &authv1.CreateUserRequest{Email: "driver@example.com", Password: "password123", FullName: "Driver", Role: authv1.Role_ROLE_DRIVER}

	_, err := c.CreateUser(context.Background(), req)
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "sin identidad propagada")

	_, err = c.CreateUser(asCaller("u-9", "OPERATOR"), req)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	created, err := c.CreateUser(asCaller("u-1", "ADMIN"), req)
	require.NoError(t, err)
	assert.Equal(t, authv1.Role_ROLE_DRIVER, created.GetUser().GetRole())

	_, err = c.CreateUser(asCaller("u-1", "ADMIN"), req)
	assert.Equal(t, codes.AlreadyExists, status.Code(err))

	bad := &authv1.CreateUserRequest{Email: "x@example.com", Password: "password123", FullName: "X"}
	_, err = c.CreateUser(asCaller("u-1", "ADMIN"), bad)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetUser(t *testing.T) {
	c := newClient(t)
	got, err := c.GetUser(asCaller("u-1", "ADMIN"), &authv1.GetUserRequest{Id: "u-1"})
	require.NoError(t, err)
	assert.Equal(t, "admin@example.com", got.GetUser().GetEmail())

	_, err = c.GetUser(asCaller("u-1", "ADMIN"), &authv1.GetUserRequest{Id: "u-404"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = c.GetUser(asCaller("u-7", "DRIVER"), &authv1.GetUserRequest{Id: "u-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
