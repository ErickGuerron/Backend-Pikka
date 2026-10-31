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

// fakeCustomers solo implementa las lecturas que usan los casos de uso.
type fakeCustomers struct {
	ports.CustomerRepository
	byUser map[uuid.UUID]domain.Customer
	byID   map[uuid.UUID]domain.Customer
}

func (f *fakeCustomers) GetByUserID(_ context.Context, userID uuid.UUID) (domain.Customer, error) {
	c, ok := f.byUser[userID]
	if !ok {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	return c, nil
}

func (f *fakeCustomers) GetByID(_ context.Context, id uuid.UUID) (domain.Customer, error) {
	c, ok := f.byID[id]
	if !ok {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	return c, nil
}

// fakeZones cuenta las llamadas para verificar cuÃ¡ndo no se consulta la zona.
type fakeZones struct {
	zoneID string
	err    error
	calls  int
}

func (f *fakeZones) ClassifyPoint(_ context.Context, _, _ float64) (string, error) {
	f.calls++
	return f.zoneID, f.err
}

type fakeGeocoder struct {
	found bool
	err   error
	calls int
}

func (f *fakeGeocoder) Locate(_ context.Context, _ string) (float64, float64, bool, error) {
	f.calls++
	if f.err != nil {
		return 0, 0, false, f.err
	}
	return -0.18, -78.48, f.found, nil
}

// fakeOrders guarda en memoria, como la tabla: pedidos y avisos.
type fakeOrders struct {
	saved   []domain.Order
	notices []domain.Notification
}

func (f *fakeOrders) Create(_ context.Context, o domain.Order, notice *domain.Notification) (domain.Order, error) {
	o.ID = uuid.NewString()
	f.saved = append(f.saved, o)
	if notice != nil {
		f.notices = append(f.notices, *notice)
	}
	return o, nil
}

func (f *fakeOrders) ListByCustomer(_ context.Context, customerID uuid.UUID) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range f.saved {
		if o.CustomerID == customerID.String() {
			out = append(out, o)
		}
	}
	return out, nil
}

// AdvanceRouteStop is not exercised by the create/list tests; it exists so fakeOrders keeps
// satisfying ports.OrderRepository.
func (f *fakeOrders) AdvanceRouteStop(_ context.Context, _, _ string, _ func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	return domain.Order{}, domain.ErrOrderNotFound
}

func (f *fakeOrders) ListAll(_ context.Context, filter ports.OrderFilter) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range f.saved {
		if filter.Status != "" && o.Status != filter.Status {
			continue
		}
		if filter.DeliveryDate != "" && o.DeliveryDate.String() != filter.DeliveryDate {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func (f *fakeOrders) ListExceptions(_ context.Context) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range f.saved {
		if o.Status == domain.StatusPendingReview {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeOrders) Review(_ context.Context, _ string, _ func(domain.Order) (domain.Order, *domain.Notification, error)) (domain.Order, error) {
	return domain.Order{}, errors.New("not used by create tests")
}

// fakeCatalog devuelve los productos de muestra por id, como el catÃ¡logo de la base.
type fakeCatalog struct {
	ports.CatalogRepository
	products map[uuid.UUID]domain.Product
}

func (f *fakeCatalog) GetProduct(_ context.Context, id uuid.UUID) (domain.Product, error) {
	p, ok := f.products[id]
	if !ok {
		return domain.Product{}, domain.ErrProductNotFound
	}
	return p, nil
}

// fakePlacer guarda el pedido como la base: en la tabla de fakeOrders, y registra sus lÃ­neas.
type fakePlacer struct {
	orders *fakeOrders
	lines  [][]domain.OrderLine
}

func (f *fakePlacer) PlaceOrder(ctx context.Context, order domain.Order, lines []domain.OrderLine, notice *domain.Notification, _ *uuid.UUID) (domain.Order, error) {
	f.lines = append(f.lines, lines)
	return f.orders.Create(ctx, order, notice)
}

var (
	customerUser = uuid.New()
	customerID   = uuid.New()
	zoneUUID     = uuid.NewString()
	localZone    = time.FixedZone("operating", -5*60*60)
	appleID      = uuid.New()
)

// appleProduct es un producto activo del catÃ¡logo de muestra, a 0.50 USD.
func appleProduct() domain.Product {
	return domain.Product{ID: appleID, Name: "Manzana", UnitPrice: decimal.RequireFromString("0.50"), Active: true}
}

func activeCustomer() domain.Customer {
	return domain.Customer{ID: customerID, UserID: customerUser, Status: domain.StatusActive, PendingSurcharge: decimal.Zero}
}

// blockedCustomer is blocked for incidents: its orders still reach the operator's review queue.
func blockedCustomer() domain.Customer {
	c := activeCustomer()
	c.Status = domain.StatusBlockedIncidents
	return c
}

// balanceBlockedCustomer is blocked for an unpaid balance: it cannot create orders (ADR 0006, section 5).
func balanceBlockedCustomer() domain.Customer {
	c := activeCustomer()
	c.Status = domain.StatusBlockedSurcharge
	return c
}

func addressCommand() Command {
	return Command{
		UserID:       customerUser,
		Role:         RoleCustomer,
		Address:      "Avenida Amazonas 123, Quito",
		DeliveryDate: "2030-04-12",
		WindowStart:  "09:00",
		WindowEnd:    "12:00",
		Items:        []LineRequest{{ProductID: appleID, Quantity: 2}},

		AddressReference: "Casa azul", PostalCode: "170150", ContactPhone: "099 123 4567",
	}
}

func knownPoint() Command {
	cmd := addressCommand()
	lat, lng := -0.18, -78.48
	cmd.Latitude, cmd.Longitude = &lat, &lng
	return cmd
}

type createFixture struct {
	uc       *CreateOrder
	orders   *fakeOrders
	zones    *fakeZones
	geocoder *fakeGeocoder
}

func newCreate(customer domain.Customer, zones *fakeZones, geocoder *fakeGeocoder) createFixture {
	orders := &fakeOrders{}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	catalog := &fakeCatalog{products: map[uuid.UUID]domain.Product{appleID: appleProduct()}}
	uc := NewCreateOrder(customers, zones, geocoder, catalog, &fakePlacer{orders: orders}, localZone)
	uc.clock = func() time.Time { return time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC) }
	return createFixture{uc: uc, orders: orders, zones: zones, geocoder: geocoder}
}

func TestCreateOrderCoveredActiveCustomerIsPlannable(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	got, err := f.uc.Run(context.Background(), knownPoint())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPending || len(got.ReviewReasons) != 0 {
		t.Fatalf("status = %s reasons = %v, want PENDING without reasons", got.Status, got.ReviewReasons)
	}
	if got.ZoneID != zoneUUID || got.CustomerID != customerID.String() {
		t.Fatalf("zone = %q customer = %q", got.ZoneID, got.CustomerID)
	}
	if f.geocoder.calls != 0 {
		t.Fatal("el punto del mapa no debe geocodificarse de nuevo")
	}
	if len(f.orders.notices) != 0 {
		t.Fatal("un pedido sin revisiÃ³n no genera aviso")
	}
}

func TestCreateOrderStoresNormalizedContactDetails(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	got, err := f.uc.Run(context.Background(), knownPoint())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.AddressReference != "Casa azul" || got.PostalCode != "170150" || got.ContactPhone != "0991234567" {
		t.Fatalf("contact details = %q %q %q", got.AddressReference, got.PostalCode, got.ContactPhone)
	}
}

func TestCreateOrderRejectsMissingContactDetailsWithoutSaving(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Command)
		want   error
	}{
		{"reference", func(c *Command) { c.AddressReference = "" }, domain.ErrInvalidAddressReference},
		{"phone", func(c *Command) { c.ContactPhone = "" }, domain.ErrInvalidPhone},
		{"postal code", func(c *Command) { c.PostalCode = "x" }, domain.ErrInvalidPostalCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})
			cmd := knownPoint()
			tt.change(&cmd)
			if _, err := f.uc.Run(context.Background(), cmd); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
			if len(f.orders.saved) != 0 {
				t.Fatal("an invalid order must not be saved")
			}
		})
	}
}

func TestCreateOrderGeocodesAddressWhenNoPointIsGiven(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	got, err := f.uc.Run(context.Background(), addressCommand())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.geocoder.calls != 1 {
		t.Fatalf("geocoder calls = %d, want 1", f.geocoder.calls)
	}
	if got.Status != domain.StatusPending || got.Latitude == 0 {
		t.Fatalf("status = %s lat = %v, want PENDING with the geocoded point", got.Status, got.Latitude)
	}
}

func TestCreateOrderUnlocatedAddressGoesToReviewWithoutZoneCheck(t *testing.T) {
	zones := &fakeZones{zoneID: zoneUUID}
	f := newCreate(activeCustomer(), zones, &fakeGeocoder{found: false})

	got, err := f.uc.Run(context.Background(), addressCommand())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPendingReview || len(got.ReviewReasons) != 1 || got.ReviewReasons[0] != domain.ReviewAddressNotLocated {
		t.Fatalf("status = %s reasons = %v, want PENDING_REVIEW [ADDRESS_NOT_LOCATED]", got.Status, got.ReviewReasons)
	}
	if zones.calls != 0 {
		t.Fatal("sin coordenadas no se consulta la zona")
	}
	if len(f.orders.notices) != 1 || f.orders.notices[0].Kind != domain.NotificationInReview {
		t.Fatalf("notices = %v, want one ORDER_IN_REVIEW", f.orders.notices)
	}
}

func TestCreateOrderOutsideZoneGoesToReviewAndNotifiesCustomer(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{err: domain.ErrZoneNotCovered}, &fakeGeocoder{found: true})

	got, err := f.uc.Run(context.Background(), addressCommand())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPendingReview || len(got.ReviewReasons) != 1 || got.ReviewReasons[0] != domain.ReviewZoneNotCovered {
		t.Fatalf("status = %s reasons = %v, want PENDING_REVIEW [ZONE_NOT_COVERED]", got.Status, got.ReviewReasons)
	}
	if got.ZoneID != "" {
		t.Fatalf("zone id = %q, want empty outside every zone", got.ZoneID)
	}
	if len(f.orders.notices) != 1 || f.orders.notices[0].Detail != "ZONE_NOT_COVERED" {
		t.Fatalf("notices = %v, want the review reason as detail", f.orders.notices)
	}
}

func TestCreateOrderBlockedCustomerGoesToReviewEvenInsideZone(t *testing.T) {
	f := newCreate(blockedCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	got, err := f.uc.Run(context.Background(), knownPoint())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPendingReview || len(got.ReviewReasons) != 1 || got.ReviewReasons[0] != domain.ReviewCustomerBlocked {
		t.Fatalf("status = %s reasons = %v, want PENDING_REVIEW [CUSTOMER_BLOCKED]", got.Status, got.ReviewReasons)
	}
}

func TestCreateOrderBlockedCustomerOutsideZoneKeepsBothReasons(t *testing.T) {
	f := newCreate(blockedCustomer(), &fakeZones{err: domain.ErrZoneNotCovered}, &fakeGeocoder{found: true})

	got, err := f.uc.Run(context.Background(), knownPoint())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.ReviewReasons) != 2 {
		t.Fatalf("reasons = %v, want both CUSTOMER_BLOCKED and ZONE_NOT_COVERED", got.ReviewReasons)
	}
}

func TestCreateOrderFailsWithoutSavingWhenZoneServiceIsDown(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{err: domain.ErrZonesUnavailable}, &fakeGeocoder{found: true})

	_, err := f.uc.Run(context.Background(), knownPoint())
	if !errors.Is(err, domain.ErrZonesUnavailable) {
		t.Fatalf("err = %v, want ErrZonesUnavailable", err)
	}
	if len(f.orders.saved) != 0 {
		t.Fatalf("saved %d orders while validation was unavailable", len(f.orders.saved))
	}
}

func TestCreateOrderFailsWithoutSavingWhenGeocoderIsDown(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{err: domain.ErrGeocoderUnavailable})

	_, err := f.uc.Run(context.Background(), addressCommand())
	if !errors.Is(err, domain.ErrGeocoderUnavailable) {
		t.Fatalf("err = %v, want ErrGeocoderUnavailable", err)
	}
	if len(f.orders.saved) != 0 {
		t.Fatal("an order was saved while the address could not be validated")
	}
}

func TestCreateOrderRejectsNonCustomerRole(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	cmd := knownPoint()
	cmd.Role = "ADMIN"
	_, err := f.uc.Run(context.Background(), cmd)
	if !errors.Is(err, domain.ErrNotCustomer) {
		t.Fatalf("err = %v, want ErrNotCustomer", err)
	}
	if len(f.orders.saved) != 0 {
		t.Fatal("a non-customer order was saved")
	}
}

func TestCreateOrderRequiresRegisteredCustomer(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	cmd := knownPoint()
	cmd.UserID = uuid.New()
	_, err := f.uc.Run(context.Background(), cmd)
	if !errors.Is(err, domain.ErrCustomerNotFound) {
		t.Fatalf("err = %v, want ErrCustomerNotFound", err)
	}
}

func TestListMyOrdersReturnsOnlyTheCallersOrders(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})
	if _, err := f.uc.Run(context.Background(), knownPoint()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Pedido de otro cliente: no debe aparecer en la lista del primero.
	f.orders.saved = append(f.orders.saved, domain.Order{CustomerID: uuid.NewString(), Status: domain.StatusPending})

	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customerUser: activeCustomer()}}
	list := NewListMyOrders(customers, f.orders)
	got, err := list.Run(context.Background(), Command{UserID: customerUser, Role: RoleCustomer})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 || got[0].CustomerID != customerID.String() {
		t.Fatalf("got %d orders, want only the caller's one", len(got))
	}
}

func (f *fakeOrders) CancelForCustomer(_ context.Context, _ string, _ uuid.UUID, _ func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	return domain.Order{}, domain.ErrOrderNotFound
}

func (f *fakeOrders) GetForCustomer(_ context.Context, _ string, _ uuid.UUID) (domain.Order, error) {
	return domain.Order{}, domain.ErrOrderNotFound
}

// ADR 0006 sección 5: la cuenta bloqueada por saldo no puede crear pedidos. Se rechaza antes de tocar
// nada: ni geocodificacion, ni zona, ni pedido guardado.
func TestCreateOrderBalanceBlockedCustomerIsRejectedWithoutSideEffects(t *testing.T) {
	for name, zones := range map[string]*fakeZones{
		"inside the zone":  {zoneID: zoneUUID},
		"outside the zone": {err: domain.ErrZoneNotCovered},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCreate(balanceBlockedCustomer(), zones, &fakeGeocoder{found: true})

			_, err := f.uc.Run(context.Background(), knownPoint())

			if !errors.Is(err, domain.ErrCustomerBlockedBySurcharge) {
				t.Fatalf("err = %v, want ErrCustomerBlockedBySurcharge", err)
			}
			if len(f.orders.saved) != 0 {
				t.Fatalf("saved %d orders, want none", len(f.orders.saved))
			}
			if f.geocoder.calls != 0 || zones.calls != 0 {
				t.Fatalf("geocoder calls = %d, zone calls = %d, want none", f.geocoder.calls, zones.calls)
			}
		})
	}
}

// ADR 0006 seccion 4: lo que debe el cliente aparece como una linea aparte, junto al pedido que confirma.
func TestCreateOrderReturnsWhatTheCustomerOwesAsASeparateLine(t *testing.T) {
	customer := activeCustomer()
	customer.PendingSurcharge = decimal.RequireFromString("7.50")
	f := newCreate(customer, &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	placed, err := f.uc.Place(context.Background(), knownPoint())
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if placed.PendingSurcharge.StringFixed(2) != "7.50" {
		t.Fatalf("pending surcharge = %s, want 7.50", placed.PendingSurcharge)
	}
	if placed.Total.Equal(placed.Total.Add(placed.PendingSurcharge)) {
		t.Fatal("the balance must not be mixed into the order total")
	}
}

func TestCreateOrderWithNothingOwedReturnsZero(t *testing.T) {
	f := newCreate(activeCustomer(), &fakeZones{zoneID: zoneUUID}, &fakeGeocoder{found: true})

	placed, err := f.uc.Place(context.Background(), knownPoint())

	if err != nil || !placed.PendingSurcharge.IsZero() {
		t.Fatalf("pending surcharge = %s, err = %v", placed.PendingSurcharge, err)
	}
}
