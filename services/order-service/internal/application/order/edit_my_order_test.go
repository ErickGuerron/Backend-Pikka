package order

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type editableOrderRepo struct {
	order domain.Order
}

func (r *editableOrderRepo) EditForCustomer(_ context.Context, _ string, _ uuid.UUID, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	next, err := decide(r.order)
	if err == nil {
		r.order = next
	}
	return next, err
}

func TestCustomerEditPreservesOriginalWhenAddressIsOutsideCoverage(t *testing.T) {
	address, lat, lon := "Nueva dirección 123", -0.2, -78.5
	current := domain.Order{ID: uuid.NewString(), Status: domain.StatusPending, Address: "Dirección original 456", ZoneID: "original"}
	repo := &editableOrderRepo{order: current}
	customers := &queueCustomers{customer: domain.Customer{ID: uuid.New()}}
	zones := &fakeZones{err: domain.ErrZoneNotCovered}
	useCase := NewEditMyOrder(customers, repo, zones, time.UTC, nil)
	edit := domain.CustomerOrderEdit{Address: &address, Latitude: &lat, Longitude: &lon}

	_, err := useCase.Run(context.Background(), uuid.New(), RoleCustomer, current.ID, edit)
	require.ErrorIs(t, err, domain.ErrZoneNotCovered)
	require.Equal(t, current, repo.order)

	zones.err, zones.zoneID = nil, "new-zone"
	updated, err := useCase.Run(context.Background(), uuid.New(), RoleCustomer, current.ID, edit)
	require.NoError(t, err)
	require.Equal(t, address, updated.Address)
	require.Equal(t, "new-zone", updated.ZoneID)
}
