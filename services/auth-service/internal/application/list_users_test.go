package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// fakeListUsersRepo es un repositorio mínimo: solo implementa lo que el caso de
// uso ListUsers necesita. Reusa fakeRepo del resto de tests para el resto de
// métodos (la interfaz UserRepository exige Create, FindByEmail, FindByID, Count
// además de List).
type fakeListUsersRepo struct {
	*fakeRepo
	list []domain.User
	err  error
}

func (f *fakeListUsersRepo) List(_ context.Context) ([]domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func newListUsersRepo(users ...domain.User) *fakeListUsersRepo {
	return &fakeListUsersRepo{fakeRepo: newFakeRepo(), list: users}
}

func sampleUsers() []domain.User {
	return []domain.User{
		{
			ID:        "u-admin",
			Email:     "admin@example.com",
			FullName:  "Admin",
			Role:      domain.RoleAdmin,
			Active:    true,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "u-customer",
			Email:     "cliente@example.com",
			FullName:  "Cliente",
			Role:      domain.RoleCustomer,
			Active:    true,
			CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "u-driver",
			Email:     "driver@example.com",
			FullName:  "Driver",
			Role:      domain.RoleDriver,
			Active:    true,
			CreatedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		},
	}
}

func TestListUsers_AsAdmin_ReturnsAll(t *testing.T) {
	repo := newListUsersRepo(sampleUsers()...)
	uc := application.NewListUsers(repo)

	got, err := uc.Execute(context.Background(), domain.RoleAdmin)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestListUsers_AsCustomer_ReturnsForbidden(t *testing.T) {
	repo := newListUsersRepo()
	uc := application.NewListUsers(repo)

	_, err := uc.Execute(context.Background(), domain.RoleCustomer)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrForbidden),
		"esperaba ErrForbidden, obtuve %v", err)
}

func TestListUsers_AsOperator_ReturnsForbidden(t *testing.T) {
	repo := newListUsersRepo()
	uc := application.NewListUsers(repo)

	_, err := uc.Execute(context.Background(), domain.RoleOperator)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrForbidden),
		"esperaba ErrForbidden, obtuve %v", err)
}

func TestListUsers_AsDriver_ReturnsForbidden(t *testing.T) {
	repo := newListUsersRepo()
	uc := application.NewListUsers(repo)

	_, err := uc.Execute(context.Background(), domain.RoleDriver)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrForbidden),
		"esperaba ErrForbidden, obtuve %v", err)
}

func TestListUsers_EmptyRepository_ReturnsEmptySlice(t *testing.T) {
	repo := newListUsersRepo()
	uc := application.NewListUsers(repo)

	got, err := uc.Execute(context.Background(), domain.RoleAdmin)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListUsers_RepositoryError_Propagates(t *testing.T) {
	repo := newListUsersRepo()
	repo.err = errors.New("db down")
	uc := application.NewListUsers(repo)

	_, err := uc.Execute(context.Background(), domain.RoleAdmin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
