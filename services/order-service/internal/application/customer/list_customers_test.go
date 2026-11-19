package customer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func seedCustomers(t *testing.T, repo *fakeCustomerRepository, n int, st domain.CustomerStatus) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := repo.Create(context.Background(), domain.Customer{
			UserID: uuid.New(), Email: "c@example.com", FullName: "C", Status: st,
		})
		require.NoError(t, err)
	}
}

func TestListCustomers_DefaultLimitAndTotal(t *testing.T) {
	repo := newFakeCustomerRepository()
	seedCustomers(t, repo, 3, domain.StatusActive)
	got, total, err := NewListCustomers(repo).Run(context.Background(), "", 0, 0)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, 3, total)
	assert.Equal(t, DefaultListLimit, repo.lastFilter.Limit)
}

func TestListCustomers_LimitIsCapped(t *testing.T) {
	repo := newFakeCustomerRepository()
	_, _, err := NewListCustomers(repo).Run(context.Background(), "", 1000, 0)
	require.NoError(t, err)
	assert.Equal(t, MaxListLimit, repo.lastFilter.Limit)
}

func TestListCustomers_NegativePagination_Invalid(t *testing.T) {
	uc := NewListCustomers(newFakeCustomerRepository())
	_, _, err := uc.Run(context.Background(), "", -1, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidPagination)
	_, _, err = uc.Run(context.Background(), "", 10, -1)
	assert.ErrorIs(t, err, domain.ErrInvalidPagination)
}

func TestListCustomers_StatusFilter(t *testing.T) {
	repo := newFakeCustomerRepository()
	seedCustomers(t, repo, 2, domain.StatusActive)
	seedCustomers(t, repo, 1, domain.StatusBlockedSurcharge)
	got, total, err := NewListCustomers(repo).Run(context.Background(), "BLOQUEADO_POR_SALDO", 10, 0)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, 1, total)
}

func TestListCustomers_InvalidStatus(t *testing.T) {
	_, _, err := NewListCustomers(newFakeCustomerRepository()).Run(context.Background(), "NOPE", 10, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidStatus)
}
