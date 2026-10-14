package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// newCreateWithCatalog arma el caso de uso con un catálogo a medida, para probar productos inactivos o ausentes.
func newCreateWithCatalog(products map[uuid.UUID]domain.Product) (*CreateOrder, *fakeOrders, *fakePlacer) {
	orders := &fakeOrders{}
	placer := &fakePlacer{orders: orders}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customerUser: activeCustomer()}}
	uc := NewCreateOrder(customers, &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true},
		&fakeCatalog{products: products}, placer, localZone)
	uc.clock = func() time.Time { return time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC) }
	return uc, orders, placer
}

var _ ports.CatalogRepository = (*fakeCatalog)(nil)

func TestPedidoLlevaSusLineasConPrecioDelCatalogo(t *testing.T) {
	uc, orders, placer := newCreateWithCatalog(map[uuid.UUID]domain.Product{appleID: appleProduct()})

	_, err := uc.Run(context.Background(), knownPoint())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(placer.lines) != 1 || len(placer.lines[0]) != 1 {
		t.Fatalf("lines = %v, want one line", placer.lines)
	}
	line := placer.lines[0][0]
	if line.Quantity != 2 || !line.UnitPrice.Equal(decimal.RequireFromString("0.50")) {
		t.Fatalf("line = %+v, want quantity 2 at 0.50", line)
	}
	if len(orders.saved) != 1 {
		t.Fatalf("saved orders = %d, want 1", len(orders.saved))
	}
}

func TestPedidoSinProductosNoSeGuarda(t *testing.T) {
	uc, orders, _ := newCreateWithCatalog(map[uuid.UUID]domain.Product{appleID: appleProduct()})
	cmd := knownPoint()
	cmd.Items = nil

	_, err := uc.Run(context.Background(), cmd)

	if !errors.Is(err, domain.ErrNoItems) {
		t.Fatalf("err = %v, want ErrNoItems", err)
	}
	if len(orders.saved) != 0 {
		t.Fatal("un pedido sin productos no debe guardarse")
	}
}

func TestProductoDesconocidoOInactivoNoSeGuarda(t *testing.T) {
	inactive := appleProduct()
	inactive.Active = false
	tests := []struct {
		name     string
		products map[uuid.UUID]domain.Product
		want     error
	}{
		{name: "producto inexistente", products: map[uuid.UUID]domain.Product{}, want: domain.ErrProductNotFound},
		{name: "producto inactivo", products: map[uuid.UUID]domain.Product{appleID: inactive}, want: domain.ErrProductInactive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, orders, _ := newCreateWithCatalog(tt.products)

			_, err := uc.Run(context.Background(), knownPoint())

			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if len(orders.saved) != 0 {
				t.Fatal("un pedido con productos no válidos no debe guardarse")
			}
		})
	}
}

func TestCantidadInvalidaOProductoRepetidoSeRechaza(t *testing.T) {
	uc, _, _ := newCreateWithCatalog(map[uuid.UUID]domain.Product{appleID: appleProduct()})

	cmd := knownPoint()
	cmd.Items = []LineRequest{{ProductID: appleID, Quantity: domain.MaxLineQuantity + 1}}
	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrInvalidQuantity) {
		t.Fatalf("quantity err = %v, want ErrInvalidQuantity", err)
	}

	cmd.Items = []LineRequest{{ProductID: appleID, Quantity: 1}, {ProductID: appleID, Quantity: 2}}
	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrDuplicateLine) {
		t.Fatalf("duplicate err = %v, want ErrDuplicateLine", err)
	}
}

// Un cliente bloqueado sigue pudiendo pedir: el pedido queda en revisión y el stock se reserva igual.
func TestPedidoEnRevisionTambienLlevaLineas(t *testing.T) {
	orders := &fakeOrders{}
	placer := &fakePlacer{orders: orders}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customerUser: blockedCustomer()}}
	uc := NewCreateOrder(customers, &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true},
		&fakeCatalog{products: map[uuid.UUID]domain.Product{appleID: appleProduct()}}, placer, localZone)
	uc.clock = func() time.Time { return time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC) }

	got, err := uc.Run(context.Background(), knownPoint())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPendingReview {
		t.Fatalf("status = %s, want PENDING_REVIEW", got.Status)
	}
	if len(placer.lines) != 1 || len(placer.lines[0]) != 1 {
		t.Fatalf("lines = %v, want one line", placer.lines)
	}
	if len(orders.notices) != 1 {
		t.Fatalf("notices = %d, want the review notice", len(orders.notices))
	}
}
