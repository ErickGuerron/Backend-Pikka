package customer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestGetByID_Existing_ReturnsCustomer(t *testing.T) {
	repo := newFakeCustomerRepository()
	uc := NewGetByID(repo)
	id := uuid.New()
	_, err := repo.Create(context.Background(), domain.Customer{
		ID: id, UserID: uuid.New(), Email: "ada@example.com", FullName: "Ada",
		Status: domain.StatusActive,
	})
	require.NoError(t, err)

	got, err := uc.Run(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
}

func TestGetByID_Missing_ReturnsNotFound(t *testing.T) {
	repo := newFakeCustomerRepository()
	uc := NewGetByID(repo)

	_, err := uc.Run(context.Background(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrCustomerNotFound)
}
