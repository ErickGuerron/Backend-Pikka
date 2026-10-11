package customer

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// fakeCustomerRepository es un in-memory implementation de ports.CustomerRepository
// para los tests de los casos de uso. No se exporta; vive solo en _test.go.
//
// Garantiza:
//   - Unicidad de user_id (devuelve ErrCustomerAlreadyExists al insertar duplicado).
//   - Búsquedas indexadas por id y user_id.
//   - UpdateStatus respeta Valid() y devuelve ErrInvalidStatus / ErrCustomerNotFound.
type fakeCustomerRepository struct {
	mu       sync.Mutex
	byID     map[uuid.UUID]domain.Customer
	byUserID map[uuid.UUID]uuid.UUID // user_id -> customer id
}

func newFakeCustomerRepository() *fakeCustomerRepository {
	return &fakeCustomerRepository{
		byID:     map[uuid.UUID]domain.Customer{},
		byUserID: map[uuid.UUID]uuid.UUID{},
	}
}

func (f *fakeCustomerRepository) Create(_ context.Context, c domain.Customer) (domain.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byUserID[c.UserID]; ok {
		return domain.Customer{}, domain.ErrCustomerAlreadyExists
	}
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	f.byID[c.ID] = c
	f.byUserID[c.UserID] = c.ID
	return c, nil
}

func (f *fakeCustomerRepository) GetByUserID(_ context.Context, userID uuid.UUID) (domain.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byUserID[userID]
	if !ok {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	return f.byID[id], nil
}

func (f *fakeCustomerRepository) GetByID(_ context.Context, id uuid.UUID) (domain.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[id]
	if !ok {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	return c, nil
}

func (f *fakeCustomerRepository) UpdateStatus(_ context.Context, id uuid.UUID, status domain.CustomerStatus) (domain.Customer, error) {
	if !status.Valid() {
		return domain.Customer{}, domain.ErrInvalidStatus
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[id]
	if !ok {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	c.Status = status
	f.byID[id] = c
	return c, nil
}
