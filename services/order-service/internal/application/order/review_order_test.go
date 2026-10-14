package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// reviewRepo aplica la decisión sobre el pedido guardado, como lo hace la transacción real.
type reviewRepo struct {
	ports.OrderRepository
	current domain.Order
	notices []domain.Notification
}

func (r *reviewRepo) Review(_ context.Context, orderID string, decide func(domain.Order) (domain.Order, *domain.Notification, error)) (domain.Order, error) {
	if orderID != r.current.ID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	next, notice, err := decide(r.current)
	if err != nil {
		return domain.Order{}, err
	}
	r.current = next
	if notice != nil {
		r.notices = append(r.notices, *notice)
	}
	return next, nil
}

// reviewFixture deja un pedido en PENDING_REVIEW con los motivos indicados.
func reviewFixture(t *testing.T, reasons ...domain.ReviewReason) (*ReviewOrder, *reviewRepo, *fakeZones, *fakeGeocoder) {
	t.Helper()
	order, err := domain.NewOrder(domain.NewOrderInput{
		Address:      "Avenida Amazonas 123, Quito",
		Latitude:     -0.18,
		Longitude:    -78.48,
		DeliveryDate: "2030-04-12",
		WindowStart:  "09:00",
		WindowEnd:    "12:00",
	}, time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	order.ID = uuid.NewString()
	order.CustomerID = customerID.String()
	order, err = order.ApplyValidation(reasons)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	repo := &reviewRepo{current: order}
	zones := &fakeZones{zoneID: zoneUUID}
	geocoder := &fakeGeocoder{found: true}
	customers := &fakeCustomers{byID: map[uuid.UUID]domain.Customer{customerID: activeCustomer()}}
	uc := NewReviewOrder(customers, zones, geocoder, repo, localZone)
	uc.clock = func() time.Time { return time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC) }
	return uc, repo, zones, geocoder
}

func operatorCmd(action, reason string) ReviewCommand {
	return ReviewCommand{Role: RoleOperator, OrderID: "", Action: action, Reason: reason}
}

func TestReviewConfirmAcceptsNonSpatialReviewAndNotifiesCustomer(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewCustomerBlocked)
	cmd := operatorCmd(ActionConfirm, "Cliente verificado por teléfono")
	cmd.OrderID = repo.current.ID

	got, err := uc.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPending || len(got.ReviewReasons) != 0 {
		t.Fatalf("status = %s reasons = %v, want PENDING", got.Status, got.ReviewReasons)
	}
	if len(repo.notices) != 1 || repo.notices[0].Kind != domain.NotificationConfirmed {
		t.Fatalf("notices = %v, want one ORDER_CONFIRMED", repo.notices)
	}
}

func TestReviewConfirmBlockedForSpatialReasons(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewZoneNotCovered)
	cmd := operatorCmd(ActionConfirm, "Aceptar igual que está")
	cmd.OrderID = repo.current.ID

	_, err := uc.Run(context.Background(), cmd)
	if !errors.Is(err, domain.ErrSpatialReasonNotOverridable) {
		t.Fatalf("err = %v, want ErrSpatialReasonNotOverridable", err)
	}
	if repo.current.Status != domain.StatusPendingReview || len(repo.notices) != 0 {
		t.Fatal("a blocked confirmation must not change the order or notify")
	}
}

func TestReviewRejectCancelsAndNotifiesWithReason(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewZoneNotCovered)
	cmd := operatorCmd(ActionReject, "Fuera de cobertura confirmada")
	cmd.OrderID = repo.current.ID

	got, err := uc.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusCancelled || len(got.ReviewReasons) != 0 {
		t.Fatalf("status = %s, want CANCELLED", got.Status)
	}
	if len(repo.notices) != 1 || repo.notices[0].Kind != domain.NotificationRejected || repo.notices[0].Detail != "Fuera de cobertura confirmada" {
		t.Fatalf("notices = %v, want ORDER_REJECTED with the audit reason", repo.notices)
	}
}

func TestReviewCorrectAddressThatNowLocatesAndCoversBecomesPending(t *testing.T) {
	uc, repo, _, geocoder := reviewFixture(t, domain.ReviewAddressNotLocated)
	address := "Calle Germán Alemán 555, Quito"
	cmd := operatorCmd(ActionCorrect, "Dirección completada con el cliente")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{Address: &address}

	got, err := uc.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if geocoder.calls != 1 {
		t.Fatalf("geocoder calls = %d, want 1 after an address change", geocoder.calls)
	}
	if got.Status != domain.StatusPending || got.ZoneID != zoneUUID {
		t.Fatalf("status = %s zone = %q, want PENDING in the zone", got.Status, got.ZoneID)
	}
	if len(repo.notices) != 1 || repo.notices[0].Kind != domain.NotificationCorrected {
		t.Fatalf("notices = %v, want ORDER_CORRECTED", repo.notices)
	}
}

func TestReviewCorrectStillOutsideZoneStaysInReviewWithNewReasons(t *testing.T) {
	uc, repo, zones, _ := reviewFixture(t, domain.ReviewAddressNotLocated)
	zones.err = domain.ErrZoneNotCovered
	address := "Calle Germán Alemán 555, Quito"
	cmd := operatorCmd(ActionCorrect, "Dirección corregida")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{Address: &address}

	got, err := uc.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != domain.StatusPendingReview || len(got.ReviewReasons) != 1 || got.ReviewReasons[0] != domain.ReviewZoneNotCovered {
		t.Fatalf("status = %s reasons = %v, want PENDING_REVIEW [ZONE_NOT_COVERED]", got.Status, got.ReviewReasons)
	}
	if len(repo.notices) != 1 || repo.notices[0].Kind != domain.NotificationInReview {
		t.Fatalf("notices = %v, want ORDER_IN_REVIEW", repo.notices)
	}
}

func TestReviewCorrectWindowKeepsCoordinatesWithoutGeocoding(t *testing.T) {
	uc, repo, _, geocoder := reviewFixture(t, domain.ReviewCustomerBlocked)
	start := "13:00"
	end := "15:00"
	cmd := operatorCmd(ActionCorrect, "Cliente pidió otra ventana")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{WindowStart: &start, WindowEnd: &end}

	got, err := uc.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if geocoder.calls != 0 {
		t.Fatal("a window change must not geocode the address again")
	}
	if got.Window.Start() != "13:00" || got.Window.End() != "15:00" {
		t.Fatalf("window = %s-%s", got.Window.Start(), got.Window.End())
	}
}

func TestReviewCorrectWithoutFieldsIsRejected(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewCustomerBlocked)
	cmd := operatorCmd(ActionCorrect, "Sin cambios reales")
	cmd.OrderID = repo.current.ID
	cmd.Correction = &Correction{}

	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, ErrNothingToCorrect) {
		t.Fatalf("err = %v, want ErrNothingToCorrect", err)
	}
}

func TestReviewRequiresOperatorRole(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewCustomerBlocked)
	cmd := operatorCmd(ActionConfirm, "Intento de un cliente")
	cmd.OrderID = repo.current.ID
	cmd.Role = RoleCustomer

	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrNotOperator) {
		t.Fatalf("err = %v, want ErrNotOperator", err)
	}
}

func TestReviewRequiresAuditReason(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewCustomerBlocked)
	cmd := operatorCmd(ActionConfirm, "  no  ")
	cmd.OrderID = repo.current.ID

	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrReasonRequired) {
		t.Fatalf("err = %v, want ErrReasonRequired", err)
	}
}

func TestReviewUnknownActionIsRejected(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t, domain.ReviewCustomerBlocked)
	cmd := operatorCmd("DELETE", "Acción que no existe")
	cmd.OrderID = repo.current.ID

	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
}

func TestReviewOnlyAcceptsOrdersInReview(t *testing.T) {
	uc, repo, _, _ := reviewFixture(t)
	cmd := operatorCmd(ActionConfirm, "Pedido ya confirmado antes")
	cmd.OrderID = repo.current.ID

	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrOrderNotInReview) {
		t.Fatalf("err = %v, want ErrOrderNotInReview", err)
	}
}

func TestReviewUnknownOrderIsNotFound(t *testing.T) {
	uc, _, _, _ := reviewFixture(t, domain.ReviewCustomerBlocked)
	cmd := operatorCmd(ActionConfirm, "Pedido que no existe")
	cmd.OrderID = uuid.NewString()

	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
}
