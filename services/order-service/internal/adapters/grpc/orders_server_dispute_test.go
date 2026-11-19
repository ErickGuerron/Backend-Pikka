package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type disputeStore struct {
	ports.DisputeRepository
	surcharge domain.Surcharge
	open      domain.Dispute
	err       error
	calls     int
}

func (s *disputeStore) OpenDispute(_ context.Context, _, customerID string, build func(domain.Surcharge) (domain.Dispute, error)) (domain.Dispute, error) {
	s.calls++
	if s.err != nil {
		return domain.Dispute{}, s.err
	}
	sc := s.surcharge
	sc.CustomerID = customerID
	d, err := build(sc)
	d.ID = uuid.NewString()
	return d, err
}

func (s *disputeStore) ResolveDispute(_ context.Context, _ string, decide func(domain.Dispute) (domain.Dispute, error)) (domain.Dispute, error) {
	s.calls++
	if s.err != nil {
		return domain.Dispute{}, s.err
	}
	return decide(s.open)
}

func (s *disputeStore) ListOpenDisputes(context.Context) ([]domain.Dispute, error) {
	s.calls++
	return []domain.Dispute{s.open}, s.err
}

func freshOwed() domain.Surcharge {
	return domain.Surcharge{ID: uuid.NewString(), OrderID: uuid.NewString(), Reason: "CLIENT_REFUSED", Status: domain.SurchargePending,
		Amount: decimal.NewNullDecimal(decimal.RequireFromString("4.20")), CreatedAt: time.Now().Add(-10 * time.Minute)}
}

func openDisputeFixture() domain.Dispute {
	return domain.Dispute{ID: uuid.NewString(), SurchargeID: uuid.NewString(), CustomerID: uuid.NewString(), Reason: "Yo estaba en casa", Status: domain.DisputeOpen, OpenedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
}

func disputeServer(store *disputeStore) *OrderServer {
	customer := domain.Customer{ID: uuid.New(), UserID: uuid.New()}
	return NewOrderServer(OrderUseCases{
		OpenDispute:      orderapp.NewOpenDispute(payCustomers{customer: customer}, store),
		ListOpenDisputes: orderapp.NewListOpenDisputes(store),
		ResolveDispute:   orderapp.NewResolveDispute(store),
	})
}

func TestOpenDisputeReturnsTheDisputeOfTheAuthenticatedCustomer(t *testing.T) {
	res, err := disputeServer(&disputeStore{surcharge: freshOwed()}).OpenDispute(asRole("CUSTOMER"),
		&ordersv1.OpenDisputeRequest{SurchargeId: uuid.NewString(), Reason: "Yo estaba en casa y nadie tocó"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.GetDispute()
	if d.GetId() == "" || d.GetStatus() != "OPEN" || d.GetCustomerId() == "" || d.GetReason() != "Yo estaba en casa y nadie tocó" || d.GetOpenedAt() == "" || d.GetResolvedBy() != "" {
		t.Fatalf("dispute = %+v", d)
	}
}

func TestOpenDisputeErrorCodes(t *testing.T) {
	late := freshOwed()
	late.CreatedAt = time.Now().Add(-3 * time.Hour)
	cases := []struct {
		name string
		role string
		sur  domain.Surcharge
		err  error
		want codes.Code
	}{
		{"operator", "OPERATOR", freshOwed(), nil, codes.PermissionDenied},
		{"outside the window", "CUSTOMER", late, nil, codes.FailedPrecondition},
		{"unknown or somebody else's", "CUSTOMER", freshOwed(), domain.ErrSurchargeNotFound, codes.NotFound},
		{"already disputed", "CUSTOMER", freshOwed(), domain.ErrDisputeExists, codes.Aborted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := disputeServer(&disputeStore{surcharge: tc.sur, err: tc.err}).OpenDispute(asRole(tc.role),
				&ordersv1.OpenDisputeRequest{SurchargeId: uuid.NewString(), Reason: "Yo estaba en casa y nadie tocó"})
			wantCode(t, err, tc.want)
		})
	}
	_, err := disputeServer(&disputeStore{}).OpenDispute(asRole("CUSTOMER"), &ordersv1.OpenDisputeRequest{SurchargeId: uuid.NewString(), Reason: "ok"})
	wantCode(t, err, codes.InvalidArgument)
}

func TestListOpenDisputesIsForStaff(t *testing.T) {
	res, err := disputeServer(&disputeStore{open: openDisputeFixture()}).ListOpenDisputes(asRole("OPERATOR"), &ordersv1.ListOpenDisputesRequest{})
	if err != nil || len(res.GetDisputes()) != 1 || res.GetDisputes()[0].GetStatus() != "OPEN" {
		t.Fatalf("res = %v err = %v", res, err)
	}
	_, err = disputeServer(&disputeStore{open: openDisputeFixture()}).ListOpenDisputes(asRole("CUSTOMER"), &ordersv1.ListOpenDisputesRequest{})
	wantCode(t, err, codes.PermissionDenied)
}

func TestResolveDisputeStoresTheOperatorFromTheVerifiedIdentity(t *testing.T) {
	res, err := disputeServer(&disputeStore{open: openDisputeFixture()}).ResolveDispute(asRole("ADMIN"),
		&ordersv1.ResolveDisputeRequest{Id: uuid.NewString(), Decision: "IN_FAVOR", Reason: "Revisamos la ruta del repartidor"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.GetDispute()
	if d.GetStatus() != "IN_FAVOR" || d.GetResolvedBy() == "" || d.GetResolutionReason() != "Revisamos la ruta del repartidor" || d.GetResolvedAt() == "" {
		t.Fatalf("dispute = %+v", d)
	}
}

func TestResolveDisputeErrorCodes(t *testing.T) {
	done := openDisputeFixture()
	done.Status = domain.DisputeAgainst
	cases := []struct {
		name     string
		role     string
		decision string
		store    *disputeStore
		want     codes.Code
	}{
		{"customer", "CUSTOMER", "IN_FAVOR", &disputeStore{open: openDisputeFixture()}, codes.PermissionDenied},
		{"unknown decision", "ADMIN", "MAYBE", &disputeStore{open: openDisputeFixture()}, codes.InvalidArgument},
		{"already resolved", "OPERATOR", "IN_FAVOR", &disputeStore{open: done}, codes.Aborted},
		{"unknown dispute", "OPERATOR", "IN_FAVOR", &disputeStore{err: domain.ErrDisputeNotFound}, codes.NotFound},
		{"paid meanwhile", "OPERATOR", "IN_FAVOR", &disputeStore{err: domain.ErrSurchargeNotDisputable}, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := disputeServer(tc.store).ResolveDispute(asRole(tc.role),
				&ordersv1.ResolveDisputeRequest{Id: uuid.NewString(), Decision: tc.decision, Reason: "Revisamos la ruta del repartidor"})
			wantCode(t, err, tc.want)
		})
	}
}

func TestDisputeIsUnimplementedWhenNotWired(t *testing.T) {
	srv := NewOrderServer(OrderUseCases{})
	_, err := srv.OpenDispute(asRole("CUSTOMER"), &ordersv1.OpenDisputeRequest{})
	wantCode(t, err, codes.Unimplemented)
	_, err = srv.ListOpenDisputes(asRole("ADMIN"), &ordersv1.ListOpenDisputesRequest{})
	wantCode(t, err, codes.Unimplemented)
	_, err = srv.ResolveDispute(asRole("ADMIN"), &ordersv1.ResolveDisputeRequest{})
	wantCode(t, err, codes.Unimplemented)
}

func TestBalanceLinesCarryTheDisputeStateAndTheLastMomentToDispute(t *testing.T) {
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	res, err := balanceServer(domain.StatusActive,
		domain.BalanceLine{Kind: domain.BalanceLineSurcharge, ID: "a", Amount: decimal.RequireFromString("4.20"), StartedAt: start},
		domain.BalanceLine{Kind: domain.BalanceLineSurcharge, ID: "b", Amount: decimal.RequireFromString("4.20"), StartedAt: start, DisputeStatus: "OPEN"},
		domain.BalanceLine{Kind: domain.BalanceLineRecidivism, ID: "c", Amount: decimal.RequireFromString("1.00"), StartedAt: start},
	).GetMyBalance(asRole("CUSTOMER"), &ordersv1.GetMyBalanceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	lines := res.GetLines()
	if lines[0].GetDisputeStatus() != "" || lines[0].GetDisputableUntil() != start.Add(domain.DisputeWindow).Format(time.RFC3339) {
		t.Fatalf("undisputed surcharge = %+v", lines[0])
	}
	if lines[1].GetDisputeStatus() != "OPEN" || lines[1].GetDisputableUntil() != "" {
		t.Fatalf("disputed surcharge = %+v", lines[1])
	}
	if lines[2].GetDisputeStatus() != "" || lines[2].GetDisputableUntil() != "" {
		t.Fatalf("recidivism line = %+v", lines[2])
	}
}
