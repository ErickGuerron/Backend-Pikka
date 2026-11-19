package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// exceptionsRepo devuelve los pedidos en revisión o un error configurado.
type exceptionsRepo struct {
	ports.OrderRepository
	orders []domain.Order
	err    error
}

func (r exceptionsRepo) ListExceptions(context.Context) ([]domain.Order, error) {
	return r.orders, r.err
}

func TestListExceptionsIsForOperatorsOnly(t *testing.T) {
	uc := NewListExceptions(exceptionsRepo{orders: []domain.Order{{ID: "o-1"}}})

	for _, role := range []string{RoleAdmin, RoleOperator} {
		got, err := uc.Run(context.Background(), role)
		require.NoError(t, err, role)
		assert.Len(t, got, 1, role)
	}
	for _, role := range []string{RoleCustomer, "DRIVER", ""} {
		_, err := uc.Run(context.Background(), role)
		assert.ErrorIs(t, err, domain.ErrNotOperator, role)
	}
}

func TestListExceptionsPropagatesRepositoryErrors(t *testing.T) {
	boom := errors.New("db down")
	uc := NewListExceptions(exceptionsRepo{err: boom})

	_, err := uc.Run(context.Background(), RoleOperator)

	assert.ErrorIs(t, err, boom)
}

// notificationsRepo entrega los avisos guardados del cliente.
type notificationsRepo struct {
	ports.NotificationRepository
	list []domain.Notification
}

func (r notificationsRepo) ListByCustomer(context.Context, uuid.UUID) ([]domain.Notification, error) {
	return r.list, nil
}

func TestListMyNotificationsReturnsOnlyTheCustomersNotices(t *testing.T) {
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{}}
	cu := activeCustomer()
	userID := uuid.New()
	customers.byUser[userID] = cu
	notices := []domain.Notification{{ID: "n-1", Kind: domain.NotificationConfirmed}}
	uc := NewListMyNotifications(customers, notificationsRepo{list: notices})

	got, err := uc.Run(context.Background(), Command{UserID: userID, Role: RoleCustomer})

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n-1", got[0].ID)
}

func TestListMyNotificationsRejectsNonCustomersAndUnknownUsers(t *testing.T) {
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{}}
	uc := NewListMyNotifications(customers, notificationsRepo{})

	_, err := uc.Run(context.Background(), Command{UserID: uuid.New(), Role: RoleOperator})
	assert.ErrorIs(t, err, domain.ErrNotCustomer)

	_, err = uc.Run(context.Background(), Command{UserID: uuid.New(), Role: RoleCustomer})
	assert.ErrorIs(t, err, domain.ErrCustomerNotFound)
}

// correctionFixture deja un pedido en revisión por ventana con un cliente activo y devuelve sus piezas.
func correctionFixture(t *testing.T) (*ReviewOrder, *reviewRepo, *fakeGeocoder) {
	t.Helper()
	uc, repo, _, geocoder := reviewFixture(t, domain.ReviewInvalidWindow)
	return uc, repo, geocoder
}

func TestReviewCorrectWithEmptyCorrectionIsRejected(t *testing.T) {
	uc, repo, _ := correctionFixture(t)
	cmd := operatorCmd(ActionCorrect, "Ajuste sin cambios reales")
	cmd.OrderID = repo.current.ID

	_, err := uc.Run(context.Background(), cmd)

	assert.ErrorIs(t, err, ErrNothingToCorrect)
	assert.Empty(t, repo.notices)

	cmd.Correction = &Correction{}
	_, err = uc.Run(context.Background(), cmd)
	assert.ErrorIs(t, err, ErrNothingToCorrect, "un bloque vacío tampoco corrige nada")
}

func TestReviewCorrectWithReversedWindowIsRejectedAndNotSaved(t *testing.T) {
	uc, repo, _ := correctionFixture(t)
	start := "13:00"
	cmd := operatorCmd(ActionCorrect, "Ventana acordada con el cliente")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{WindowStart: &start}

	_, err := uc.Run(context.Background(), cmd)

	assert.ErrorIs(t, err, domain.ErrInvalidWindow)
	assert.Equal(t, domain.StatusPendingReview, repo.current.Status)
	assert.Empty(t, repo.notices)
}

func TestReviewCorrectChangingAddressUsesTheGeocoder(t *testing.T) {
	uc, repo, geocoder := correctionFixture(t)
	address := "Calle Germán Alemán 555, Quito"
	cmd := operatorCmd(ActionCorrect, "Dirección completada con el cliente")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{Address: &address}

	_, err := uc.Run(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, 1, geocoder.calls, "una dirección nueva se vuelve a localizar")
}

func TestReviewCorrectGeocoderFailureLeavesOrderUntouched(t *testing.T) {
	uc, repo, geocoder := correctionFixture(t)
	geocoder.err = errors.New("geocoder down")
	address := "Calle Germán Alemán 555, Quito"
	cmd := operatorCmd(ActionCorrect, "Dirección completada con el cliente")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{Address: &address}

	_, err := uc.Run(context.Background(), cmd)

	assert.Error(t, err)
	assert.Equal(t, domain.StatusPendingReview, repo.current.Status)
	assert.Empty(t, repo.notices)
}

func TestReviewCorrectFailsWhenCustomerIsUnknown(t *testing.T) {
	uc, repo, _ := correctionFixture(t)
	uc.customers = &fakeCustomers{byID: map[uuid.UUID]domain.Customer{}}
	end := "11:00"
	cmd := operatorCmd(ActionCorrect, "Ventana acordada con el cliente")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{WindowEnd: &end}

	_, err := uc.Run(context.Background(), cmd)

	assert.ErrorIs(t, err, domain.ErrCustomerNotFound)
	assert.Empty(t, repo.notices)
}

func TestReviewCorrectFailsOnMalformedCustomerID(t *testing.T) {
	uc, repo, _ := correctionFixture(t)
	repo.current.CustomerID = "no-es-uuid"
	end := "11:00"
	cmd := operatorCmd(ActionCorrect, "Ventana acordada con el cliente")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{WindowEnd: &end}

	_, err := uc.Run(context.Background(), cmd)

	assert.Error(t, err)
	assert.Empty(t, repo.notices)
}
