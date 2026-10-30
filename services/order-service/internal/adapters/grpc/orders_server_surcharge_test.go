package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type surchargeStore struct {
	ports.SurchargeRepository
	stored domain.Surcharge
}

func (s *surchargeStore) ResolveSurchargeVerification(_ context.Context, id string, decide func(domain.Surcharge) (domain.Surcharge, error)) (domain.Surcharge, error) {
	if id != s.stored.ID {
		return domain.Surcharge{}, domain.ErrSurchargeNotFound
	}
	next, err := decide(s.stored)
	if err == nil {
		s.stored = next
	}
	return next, err
}

func (s *surchargeStore) ListSurchargesPendingVerification(context.Context) ([]domain.Surcharge, error) {
	return []domain.Surcharge{s.stored}, nil
}

func surchargeServer(store *surchargeStore) *OrderServer {
	return NewOrderServer(OrderUseCases{Surcharges: orderapp.NewResolveSurchargeVerification(store)})
}

func asRole(role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(metadataUserID, uuid.NewString(), metadataUserRole, role))
}

func waitingStore() *surchargeStore {
	return &surchargeStore{stored: domain.Surcharge{ID: uuid.NewString(), CustomerID: uuid.NewString(), OrderID: uuid.NewString(), Reason: "WRONG_ADDRESS", Status: domain.SurchargePendingVerification, CreatedAt: time.Now()}}
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (%v), want %v", got, err, want)
	}
}

func TestResolveSurchargeVerificationAsOperator(t *testing.T) {
	store := waitingStore()
	res, err := surchargeServer(store).ResolveSurchargeVerification(asRole("OPERATOR"), &ordersv1.ResolveSurchargeVerificationRequest{
		Id: store.stored.ID, Decision: "WAIVE", Reason: "La direccion era del negocio",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := res.GetSurcharge()
	if got.GetStatus() != "WAIVED" || got.GetResolutionReason() != "La direccion era del negocio" || got.GetResolvedBy() == "" || got.GetResolvedAt() == "" {
		t.Fatalf("surcharge = %+v", got)
	}
}

func TestResolveSurchargeVerificationErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		req  func(id string) *ordersv1.ResolveSurchargeVerificationRequest
		want codes.Code
	}{
		{"customer", asRole("CUSTOMER"), func(id string) *ordersv1.ResolveSurchargeVerificationRequest {
			return &ordersv1.ResolveSurchargeVerificationRequest{Id: id, Decision: "WAIVE", Reason: "motivo valido"}
		}, codes.PermissionDenied},
		{"no identity", context.Background(), func(id string) *ordersv1.ResolveSurchargeVerificationRequest {
			return &ordersv1.ResolveSurchargeVerificationRequest{Id: id, Decision: "WAIVE", Reason: "motivo valido"}
		}, codes.Unauthenticated},
		{"unknown id", asRole("OPERATOR"), func(string) *ordersv1.ResolveSurchargeVerificationRequest {
			return &ordersv1.ResolveSurchargeVerificationRequest{Id: uuid.NewString(), Decision: "WAIVE", Reason: "motivo valido"}
		}, codes.NotFound},
		{"unknown decision", asRole("OPERATOR"), func(id string) *ordersv1.ResolveSurchargeVerificationRequest {
			return &ordersv1.ResolveSurchargeVerificationRequest{Id: id, Decision: "MAYBE", Reason: "motivo valido"}
		}, codes.InvalidArgument},
		{"short reason", asRole("OPERATOR"), func(id string) *ordersv1.ResolveSurchargeVerificationRequest {
			return &ordersv1.ResolveSurchargeVerificationRequest{Id: id, Decision: "WAIVE", Reason: "no"}
		}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := waitingStore()
			_, err := surchargeServer(store).ResolveSurchargeVerification(tc.ctx, tc.req(store.stored.ID))
			wantCode(t, err, tc.want)
		})
	}
}

func TestResolveSurchargeVerificationAlreadyResolvedIsAborted(t *testing.T) {
	store := waitingStore()
	store.stored.Status = domain.SurchargeWaived
	_, err := surchargeServer(store).ResolveSurchargeVerification(asRole("ADMIN"), &ordersv1.ResolveSurchargeVerificationRequest{
		Id: store.stored.ID, Decision: "CONFIRM", Reason: "motivo valido",
	})
	wantCode(t, err, codes.Aborted)
}

func TestResolveSurchargeVerificationNotWired(t *testing.T) {
	_, err := NewOrderServer(OrderUseCases{}).ResolveSurchargeVerification(asRole("OPERATOR"), &ordersv1.ResolveSurchargeVerificationRequest{})
	wantCode(t, err, codes.Unimplemented)
	_, err = NewOrderServer(OrderUseCases{}).ListSurchargesPendingVerification(asRole("OPERATOR"), &ordersv1.ListSurchargesPendingVerificationRequest{})
	wantCode(t, err, codes.Unimplemented)
}

func TestListSurchargesPendingVerification(t *testing.T) {
	store := waitingStore()
	store.stored.Amount = decimal.NullDecimal{Decimal: decimal.RequireFromString("1.5"), Valid: true}
	res, err := surchargeServer(store).ListSurchargesPendingVerification(asRole("OPERATOR"), &ordersv1.ListSurchargesPendingVerificationRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSurcharges()) != 1 || res.GetSurcharges()[0].GetAmount() != "1.50" || res.GetSurcharges()[0].GetStatus() != "PENDING_VERIFICATION" {
		t.Fatalf("surcharges = %+v", res.GetSurcharges())
	}
	_, err = surchargeServer(store).ListSurchargesPendingVerification(asRole("DRIVER"), &ordersv1.ListSurchargesPendingVerificationRequest{})
	wantCode(t, err, codes.PermissionDenied)
}
