package grpc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/security"
)

// incoming simula la identidad que el Gateway propaga en la metadata entrante (llamada directa, sin red).
func incoming(id, role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcadapter.MetadataUserID, id, grpcadapter.MetadataUserRole, role))
}

// covUsers envuelve el repositorio en memoria y puede fallar las lecturas.
type covUsers struct {
	ports.UserRepository
	findErr error
	listErr error
}

func (r covUsers) FindByID(ctx context.Context, id string) (domain.User, error) {
	if r.findErr != nil {
		return domain.User{}, r.findErr
	}
	return r.UserRepository.FindByID(ctx, id)
}

func (r covUsers) List(ctx context.Context) ([]domain.User, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.UserRepository.List(ctx)
}

func TestUnwiredUseCasesAreUnimplemented(t *testing.T) {
	srv := grpcadapter.NewServer(grpcadapter.UseCases{})
	ctx := incoming("u-1", "ADMIN")

	_, err := srv.SignupCustomer(ctx, &authv1.SignupCustomerRequest{Email: "x@example.com", Password: "password123", FullName: "X"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = srv.ListUsers(ctx, &authv1.ListUsersRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = srv.UpdateUser(ctx, &authv1.UpdateUserRequest{Id: "u-2"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = srv.DeactivateUser(ctx, &authv1.DeactivateUserRequest{Id: "u-2"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestListUsersReturnsEveryUserForAdminsOnly(t *testing.T) {
	ctx := context.Background()
	repo := &memRepo{}
	_, err := application.NewBootstrapAdmin(repo, security.NewBcryptHasher(bcrypt.MinCost)).Execute(ctx, "admin@example.com", "admin-password")
	require.NoError(t, err)
	_, err = repo.Create(ctx, domain.User{Email: "driver@example.com", FullName: "Driver", Role: domain.RoleDriver, Active: true})
	require.NoError(t, err)
	srv := grpcadapter.NewServer(grpcadapter.UseCases{ListUsers: application.NewListUsers(repo)})

	res, err := srv.ListUsers(incoming("u-1", "ADMIN"), &authv1.ListUsersRequest{})

	require.NoError(t, err)
	assert.Len(t, res.GetUsers(), 2)

	_, err = srv.ListUsers(incoming("u-9", "OPERATOR"), &authv1.ListUsersRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "solo ADMIN lista usuarios")
}

func TestListUsersRepositoryFailureIsInternal(t *testing.T) {
	srv := grpcadapter.NewServer(grpcadapter.UseCases{
		ListUsers: application.NewListUsers(covUsers{UserRepository: &memRepo{}, listErr: errors.New("db down")}),
	})

	_, err := srv.ListUsers(incoming("u-1", "ADMIN"), &authv1.ListUsersRequest{})

	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, "internal error", status.Convert(err).Message(), "el detalle de la base no llega al cliente")
}

func TestGetUserMapsDeadlineCancellationAndInternalErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"deadline", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"canceled", context.Canceled, codes.Canceled},
		{"unexpected", errors.New("db down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := grpcadapter.NewServer(grpcadapter.UseCases{
				GetUser: application.NewGetUser(covUsers{UserRepository: &memRepo{}, findErr: tt.err}),
			})

			_, err := srv.GetUser(incoming("u-1", "ADMIN"), &authv1.GetUserRequest{Id: "u-1"})

			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestUpdateUserNothingToUpdateIsInvalid(t *testing.T) {
	c := newClient(t)
	created, err := c.CreateUser(asCaller("u-1", "ADMIN"), &authv1.CreateUserRequest{Email: "driver@example.com", Password: "password123", FullName: "Driver", Role: authv1.Role_ROLE_DRIVER})
	require.NoError(t, err)

	_, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: created.GetUser().GetId()})

	assert.Equal(t, codes.InvalidArgument, status.Code(err), "sin campos a cambiar no hay nada que actualizar")
}

func TestUpdateUserRejectsInvalidNameAndRoleWithInvalidArgument(t *testing.T) {
	c := newClient(t)
	created, err := c.CreateUser(asCaller("u-1", "ADMIN"), &authv1.CreateUserRequest{Email: "driver@example.com", Password: "password123", FullName: "Driver", Role: authv1.Role_ROLE_DRIVER})
	require.NoError(t, err)
	blank := "   "

	_, err = c.UpdateUser(asCaller("u-1", "ADMIN"), &authv1.UpdateUserRequest{Id: created.GetUser().GetId(), FullName: &blank})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestDeactivateUserUnknownIsNotFound(t *testing.T) {
	c := newClient(t)

	_, err := c.DeactivateUser(asCaller("u-1", "ADMIN"), &authv1.DeactivateUserRequest{Id: "u-404"})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestSignupCustomerHasherFailureIsInternal(t *testing.T) {
	signup := application.NewSignupCustomer(&memRepo{}, failingHasher{}, failingTokens{})
	srv := grpcadapter.NewServer(grpcadapter.UseCases{SignupCustomer: signup})

	_, err := srv.SignupCustomer(context.Background(), &authv1.SignupCustomerRequest{Email: "x@example.com", Password: "password123", FullName: "X"})

	assert.Equal(t, codes.Internal, status.Code(err), "un fallo del hasher no se revela al cliente")
}

// failingHasher y failingTokens fallan para comprobar que el adaptador traduce errores ajenos a Internal.
type failingHasher struct{}

func (failingHasher) Hash(string) (string, error) { return "", errors.New("hasher down") }
func (failingHasher) Compare(string, string) bool { return false }

type failingTokens struct{}

func (failingTokens) Issue(string, domain.Role) (string, time.Duration, error) {
	return "", 0, errors.New("tokens down")
}
func (failingTokens) Verify(string) (ports.Claims, error) {
	return ports.Claims{}, errors.New("tokens down")
}
