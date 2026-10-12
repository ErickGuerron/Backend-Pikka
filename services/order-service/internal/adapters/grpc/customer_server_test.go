package grpc

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	customercase "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/customer"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// fakeCustomerRepository es un in-memory implementation de ports.CustomerRepository
// para los tests del adaptador gRPC. Duplicamos el fake del paquete
// `application/customer` porque ese vive en _test.go y no se exporta.
//
// Notas de diseño:
//   - Garantiza unicidad de user_id (ErrCustomerAlreadyExists).
//   - Permite búsquedas por id y por user_id.
//   - UpdateStatus respeta Valid() y devuelve ErrInvalidStatus / ErrCustomerNotFound.
//   - Para los tests de CanCreateOrders pre-registramos el Customer con
//     ID == UserID porque el caso de uso (BE-T4b) toma el parámetro como id
//     y hace repo.GetByID; en producción ese id llega por otra vía. Acá
//     emparejamos ambos UUIDs para que el test refleje el comportamiento
//     observable del handler.
type fakeCustomerRepository struct {
	mu         sync.Mutex
	byID       map[uuid.UUID]domain.Customer
	byUserID   map[uuid.UUID]uuid.UUID
	lastFilter ports.CustomerListFilter
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
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
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

func (f *fakeCustomerRepository) UpdateStatus(_ context.Context, id uuid.UUID, st domain.CustomerStatus) (domain.Customer, error) {
	if !st.Valid() {
		return domain.Customer{}, domain.ErrInvalidStatus
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[id]
	if !ok {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	c.Status = st
	f.byID[id] = c
	return c, nil
}

// compile-time check.
var _ ports.CustomerRepository = (*fakeCustomerRepository)(nil)

// newTestServer construye el server gRPC con todos los use cases cableados
// contra el fakeRepo compartido.
func newTestServer(t *testing.T, repo ports.CustomerRepository) *CustomerServer {
	t.Helper()
	return NewCustomerServer(UseCases{
		Create:          customercase.NewCreateCustomer(repo),
		GetByID:         customercase.NewGetByID(repo),
		List:            customercase.NewListCustomers(repo),
		GetByUserID:     customercase.NewGetByUserID(repo),
		UpdateStatus:    customercase.NewUpdateStatus(repo),
		CanCreateOrders: customercase.NewCanCreateOrders(repo),
	})
}

func TestCreateCustomer_Valid_ReturnsCustomer(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	userID := uuid.New()

	resp, err := srv.CreateCustomer(context.Background(), &customerv1.CreateCustomerRequest{
		UserId:   userID.String(),
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetCustomer())

	got := resp.GetCustomer()
	assert.NotEmpty(t, got.GetId())
	assert.Equal(t, userID.String(), got.GetUserId())
	assert.Equal(t, "Ada Lovelace", got.GetFullName())
	assert.Equal(t, "ada@example.com", got.GetEmail())
	assert.Equal(t, "ACTIVO", got.GetStatus())
	assert.Equal(t, "0.00", got.GetPendingSurcharge())
	assert.NotEmpty(t, got.GetCreatedAt())
	assert.NotEmpty(t, got.GetUpdatedAt())
}

func TestCreateCustomer_DuplicateUserID_ReturnsAlreadyExists(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	userID := uuid.New()

	_, err := srv.CreateCustomer(context.Background(), &customerv1.CreateCustomerRequest{
		UserId:   userID.String(),
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
	})
	require.NoError(t, err)

	_, err = srv.CreateCustomer(context.Background(), &customerv1.CreateCustomerRequest{
		UserId:   userID.String(),
		FullName: "Duplicado",
		Email:    "otro@example.com",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

func TestCreateCustomer_InvalidUUID_ReturnsInvalidArgument(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)

	_, err := srv.CreateCustomer(context.Background(), &customerv1.CreateCustomerRequest{
		UserId:   "no-es-uuid",
		FullName: "X",
		Email:    "x@example.com",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestGetByUserID_NotFound_ReturnsNotFound(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)

	_, err := srv.GetByUserID(context.Background(), &customerv1.GetByUserIDRequest{
		UserId: uuid.New().String(),
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestGetByUserID_InvalidUUID_ReturnsInvalidArgument(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)

	_, err := srv.GetByUserID(context.Background(), &customerv1.GetByUserIDRequest{
		UserId: "garbage",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestUpdateStatus_ValidStatus_Updates(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	userID := uuid.New()

	created, err := srv.CreateCustomer(context.Background(), &customerv1.CreateCustomerRequest{
		UserId:   userID.String(),
		FullName: "Ada",
		Email:    "ada@example.com",
	})
	require.NoError(t, err)

	resp, err := srv.UpdateStatus(context.Background(), &customerv1.UpdateStatusRequest{
		Id:     created.GetCustomer().GetId(),
		Status: "BLOQUEADO_POR_SALDO",
	})
	require.NoError(t, err)
	assert.Equal(t, "BLOQUEADO_POR_SALDO", resp.GetCustomer().GetStatus())
}

func TestUpdateStatus_InvalidStatus_ReturnsInvalidArgument(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	userID := uuid.New()

	created, err := srv.CreateCustomer(context.Background(), &customerv1.CreateCustomerRequest{
		UserId:   userID.String(),
		FullName: "Ada",
		Email:    "ada@example.com",
	})
	require.NoError(t, err)

	_, err = srv.UpdateStatus(context.Background(), &customerv1.UpdateStatusRequest{
		Id:     created.GetCustomer().GetId(),
		Status: "INVALIDO",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestUpdateStatus_InvalidUUID_ReturnsInvalidArgument(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)

	_, err := srv.UpdateStatus(context.Background(), &customerv1.UpdateStatusRequest{
		Id:     "no-es-uuid",
		Status: "ACTIVO",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestCanCreateOrders_Active_ReturnsTrue(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	// El caso de uso (BE-T4b) hace repo.GetByID con el UUID recibido. Para
	// reflejar eso en el test pre-registramos al Customer con ID == UserID.
	sharedID := uuid.New()
	_, err := repo.Create(context.Background(), domain.Customer{
		ID:               sharedID,
		UserID:           sharedID,
		Email:            "ada@example.com",
		FullName:         "Ada",
		Status:           domain.StatusActive,
		PendingSurcharge: mustDecimal(t, "0"),
	})
	require.NoError(t, err)

	resp, err := srv.CanCreateOrders(context.Background(), &customerv1.CanCreateOrdersRequest{
		UserId: sharedID.String(),
	})
	require.NoError(t, err)
	assert.True(t, resp.GetCanCreate())
}

func TestCanCreateOrders_Blocked_ReturnsFalse(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	sharedID := uuid.New()
	_, err := repo.Create(context.Background(), domain.Customer{
		ID:               sharedID,
		UserID:           sharedID,
		Email:            "ada@example.com",
		FullName:         "Ada",
		Status:           domain.StatusBlockedSurcharge,
		PendingSurcharge: mustDecimal(t, "50.00"),
	})
	require.NoError(t, err)

	resp, err := srv.CanCreateOrders(context.Background(), &customerv1.CanCreateOrdersRequest{
		UserId: sharedID.String(),
	})
	require.NoError(t, err)
	assert.False(t, resp.GetCanCreate())
}

func TestCanCreateOrders_NotFound_ReturnsNotFound(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)

	_, err := srv.CanCreateOrders(context.Background(), &customerv1.CanCreateOrdersRequest{
		UserId: uuid.New().String(),
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestCanCreateOrders_InvalidUUID_ReturnsInvalidArgument(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)

	_, err := srv.CanCreateOrders(context.Background(), &customerv1.CanCreateOrdersRequest{
		UserId: "basura",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// TestMapError cubre mapError indirectamente vía los caminos anteriores,
// pero dejamos un caso explícito para ErrCustomerAlreadyExists cuando el
// flujo llega desde un camino no testeado arriba.
func TestMapError_UnknownError_ReturnsInternal(t *testing.T) {
	err := mapError(errors.New("algo no previsto"))
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	require.NoError(t, err)
	return d
}

// List filtra por status, ordena de forma estable por id y pagina.
func (f *fakeCustomerRepository) List(_ context.Context, fl ports.CustomerListFilter) ([]domain.Customer, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastFilter = fl
	all := make([]domain.Customer, 0, len(f.byID))
	for _, c := range f.byID {
		if fl.Status != nil && c.Status != *fl.Status {
			continue
		}
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID.String() < all[j].ID.String() })
	total := len(all)
	if fl.Offset >= total {
		return []domain.Customer{}, total, nil
	}
	end := fl.Offset + fl.Limit
	if end > total {
		end = total
	}
	return all[fl.Offset:end], total, nil
}

func TestGetByID_Existing_ReturnsCustomer(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	c, err := repo.Create(context.Background(), domain.Customer{
		UserID: uuid.New(), Email: "ada@example.com", FullName: "Ada", Status: domain.StatusActive,
	})
	require.NoError(t, err)

	resp, err := srv.GetByID(context.Background(), &customerv1.GetByIDRequest{Id: c.ID.String()})
	require.NoError(t, err)
	assert.Equal(t, c.ID.String(), resp.GetCustomer().GetId())
}

func TestGetByID_Errors(t *testing.T) {
	srv := newTestServer(t, newFakeCustomerRepository())
	_, err := srv.GetByID(context.Background(), &customerv1.GetByIDRequest{Id: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = srv.GetByID(context.Background(), &customerv1.GetByIDRequest{Id: uuid.NewString()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestListCustomers_ReturnsPageAndTotal(t *testing.T) {
	repo := newFakeCustomerRepository()
	srv := newTestServer(t, repo)
	for i := 0; i < 3; i++ {
		_, err := repo.Create(context.Background(), domain.Customer{
			UserID: uuid.New(), Email: "a@example.com", FullName: "A", Status: domain.StatusActive,
		})
		require.NoError(t, err)
	}
	resp, err := srv.ListCustomers(context.Background(), &customerv1.ListCustomersRequest{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, resp.GetCustomers(), 2)
	assert.Equal(t, int32(3), resp.GetTotal())
}

func TestListCustomers_InvalidArguments(t *testing.T) {
	srv := newTestServer(t, newFakeCustomerRepository())
	_, err := srv.ListCustomers(context.Background(), &customerv1.ListCustomersRequest{Limit: -1})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = srv.ListCustomers(context.Background(), &customerv1.ListCustomersRequest{Status: "NOPE"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListCustomers_NotWired_Unimplemented(t *testing.T) {
	_, err := NewCustomerServer(UseCases{}).ListCustomers(context.Background(), &customerv1.ListCustomersRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
