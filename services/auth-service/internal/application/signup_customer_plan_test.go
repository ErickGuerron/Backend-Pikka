package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// Plan: "registro concurrente de clientes" con el mismo correo.
func TestPlan_SignupCustomer_ConcurrentSameEmail_ExactlyOneSucceeds(t *testing.T) {
	const callers = 16
	repo := newFakeRepo()
	uc := application.NewSignupCustomer(repo, &fakeHasher{}, fakeTokens{})

	start := make(chan struct{})
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = uc.Execute(context.Background(), "race@example.com", "password123", "Cliente Race")
		}(i)
	}
	close(start)
	wg.Wait()

	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrEmailAlreadyExists):
			dup++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, callers-1, dup)
	n, err := repo.Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestPlan_SignupCustomer_MandatoryDataOnly_Succeeds(t *testing.T) {
	uc := application.NewSignupCustomer(newFakeRepo(), &fakeHasher{}, fakeTokens{})
	res, err := uc.Execute(context.Background(), "min@example.com", "password123", "Nombre Completo")
	require.NoError(t, err)
	assert.NotEmpty(t, res.AccessToken)
	assert.Equal(t, domain.RoleCustomer, res.User.Role)
}

func TestPlan_SignupCustomer_MissingMandatoryData_Rejected(t *testing.T) {
	tests := []struct {
		name, email, password, fullName string
		want                            error
	}{
		{"nombre vacío", "a@example.com", "password123", "", domain.ErrInvalidFullName},
		{"nombre solo espacios", "a@example.com", "password123", "   ", domain.ErrInvalidFullName},
		{"cuerpo vacío", "", "", "", domain.ErrInvalidEmail},
		{"contraseña vacía", "a@example.com", "", "Nombre", domain.ErrInvalidPassword},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			uc := application.NewSignupCustomer(repo, &fakeHasher{}, fakeTokens{})
			_, err := uc.Execute(context.Background(), tc.email, tc.password, tc.fullName)
			assert.ErrorIs(t, err, tc.want)
			n, _ := repo.Count(context.Background())
			assert.Zero(t, n, "nothing must be persisted")
		})
	}
}
