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

type disputeRepoFake struct {
	ports.DisputeRepository
	surcharge domain.Surcharge
	open      domain.Dispute
	err       error
	calls     int
	opened    []domain.Dispute
	resolved  []domain.Dispute
}

func (f *disputeRepoFake) OpenDispute(_ context.Context, _, customerID string, build func(domain.Surcharge) (domain.Dispute, error)) (domain.Dispute, error) {
	f.calls++
	if f.err != nil {
		return domain.Dispute{}, f.err
	}
	s := f.surcharge
	s.CustomerID = customerID
	d, err := build(s)
	if err != nil {
		return domain.Dispute{}, err
	}
	f.opened = append(f.opened, d)
	return d, nil
}

func (f *disputeRepoFake) ResolveDispute(_ context.Context, _ string, decide func(domain.Dispute) (domain.Dispute, error)) (domain.Dispute, error) {
	f.calls++
	if f.err != nil {
		return domain.Dispute{}, f.err
	}
	d, err := decide(f.open)
	if err != nil {
		return domain.Dispute{}, err
	}
	f.resolved = append(f.resolved, d)
	return d, nil
}

func (f *disputeRepoFake) ListOpenDisputes(context.Context) ([]domain.Dispute, error) {
	f.calls++
	return []domain.Dispute{f.open}, f.err
}

type disputeCustomers struct {
	ports.CustomerRepository
	id uuid.UUID
}

func (c disputeCustomers) GetByUserID(context.Context, uuid.UUID) (domain.Customer, error) {
	return domain.Customer{ID: c.id}, nil
}

func freshSurcharge(now time.Time) domain.Surcharge {
	return domain.Surcharge{ID: "s-1", OrderID: "o-1", Reason: "CLIENT_REFUSED", Status: domain.SurchargePending,
		Amount: decimal.NewNullDecimal(decimal.RequireFromString("4.20")), CreatedAt: now.Add(-10 * time.Minute)}
}

func openCommand() OpenDisputeCommand {
	return OpenDisputeCommand{UserID: uuid.New(), Role: RoleCustomer, SurchargeID: uuid.NewString(), Reason: "Yo estaba en casa y nadie tocó"}
}

func TestOpenDisputeTakesTheCustomerFromTheVerifiedIdentity(t *testing.T) {
	customer := uuid.New()
	repo := &disputeRepoFake{}
	now := time.Now()
	repo.surcharge = freshSurcharge(now)

	got, err := NewOpenDispute(disputeCustomers{id: customer}, repo).Run(context.Background(), openCommand())

	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerID != customer.String() || got.Status != domain.DisputeOpen || got.Reason != "Yo estaba en casa y nadie tocó" {
		t.Fatalf("dispute = %+v", got)
	}
}

func TestOpenDisputeRejectsBeforeTheRepository(t *testing.T) {
	cases := map[string]struct {
		mutate func(*OpenDisputeCommand)
		want   error
	}{
		"only a customer disputes": {func(c *OpenDisputeCommand) { c.Role = RoleOperator }, domain.ErrNotCustomer},
		"surcharge id not a uuid":  {func(c *OpenDisputeCommand) { c.SurchargeID = "x" }, domain.ErrSurchargeNotFound},
		"reason too short":         {func(c *OpenDisputeCommand) { c.Reason = "ok" }, domain.ErrReasonRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &disputeRepoFake{}
			cmd := openCommand()
			tc.mutate(&cmd)

			_, err := NewOpenDispute(disputeCustomers{id: uuid.New()}, repo).Run(context.Background(), cmd)

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != 0 {
				t.Fatalf("repository called %d times", repo.calls)
			}
		})
	}
}

func TestOpenDisputeKeepsTheDomainRulesAndTheRepositoryDecisions(t *testing.T) {
	now := time.Now()
	late := freshSurcharge(now)
	late.CreatedAt = now.Add(-3 * time.Hour)
	repo := &disputeRepoFake{surcharge: late}
	if _, err := NewOpenDispute(disputeCustomers{id: uuid.New()}, repo).Run(context.Background(), openCommand()); !errors.Is(err, domain.ErrDisputeWindowClosed) {
		t.Fatalf("late err = %v", err)
	}
	for _, want := range []error{domain.ErrSurchargeNotFound, domain.ErrDisputeExists} {
		repo := &disputeRepoFake{err: want}
		if _, err := NewOpenDispute(disputeCustomers{id: uuid.New()}, repo).Run(context.Background(), openCommand()); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}
}

func openDisputeValue() domain.Dispute {
	return domain.Dispute{ID: "d-1", SurchargeID: "s-1", CustomerID: "c-1", Reason: "Yo estaba en casa", Status: domain.DisputeOpen, OpenedAt: time.Now()}
}

func resolveCommand() ResolveDisputeCommand {
	return ResolveDisputeCommand{Role: RoleOperator, OperatorID: "op-1", DisputeID: uuid.NewString(), Decision: "IN_FAVOR", Reason: "Revisamos la ruta del repartidor"}
}

func TestResolveDisputeByOperatorAndAdmin(t *testing.T) {
	for _, role := range []string{RoleOperator, RoleAdmin} {
		repo := &disputeRepoFake{open: openDisputeValue()}
		cmd := resolveCommand()
		cmd.Role = role

		got, err := NewResolveDispute(repo).Run(context.Background(), cmd)

		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if got.Status != domain.DisputeInFavor || got.ResolvedBy != "op-1" || got.ResolutionReason != "Revisamos la ruta del repartidor" {
			t.Fatalf("%s: dispute = %+v", role, got)
		}
	}
}

func TestResolveDisputeRejectsBeforeTheRepository(t *testing.T) {
	cases := map[string]struct {
		mutate func(*ResolveDisputeCommand)
		want   error
	}{
		"a customer cannot resolve": {func(c *ResolveDisputeCommand) { c.Role = RoleCustomer }, domain.ErrNotOperator},
		"dispute id not a uuid":     {func(c *ResolveDisputeCommand) { c.DisputeID = "x" }, domain.ErrDisputeNotFound},
		"unknown decision":          {func(c *ResolveDisputeCommand) { c.Decision = "MAYBE" }, domain.ErrUnknownDisputeDecision},
		"open is not a decision":    {func(c *ResolveDisputeCommand) { c.Decision = "OPEN" }, domain.ErrUnknownDisputeDecision},
		"short reason":              {func(c *ResolveDisputeCommand) { c.Reason = "ok" }, domain.ErrReasonRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &disputeRepoFake{open: openDisputeValue()}
			cmd := resolveCommand()
			tc.mutate(&cmd)

			_, err := NewResolveDispute(repo).Run(context.Background(), cmd)

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != 0 {
				t.Fatalf("repository called %d times", repo.calls)
			}
		})
	}
}

func TestResolveDisputeTwiceAndRepositoryDecisions(t *testing.T) {
	done := openDisputeValue()
	done.Status = domain.DisputeAgainst
	if _, err := NewResolveDispute(&disputeRepoFake{open: done}).Run(context.Background(), resolveCommand()); !errors.Is(err, domain.ErrDisputeAlreadyResolved) {
		t.Fatalf("second resolve err = %v", err)
	}
	for _, want := range []error{domain.ErrDisputeNotFound, domain.ErrSurchargeNotDisputable} {
		if _, err := NewResolveDispute(&disputeRepoFake{err: want}).Run(context.Background(), resolveCommand()); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}
}

func TestListOpenDisputesIsForStaffOnly(t *testing.T) {
	repo := &disputeRepoFake{open: openDisputeValue()}
	if _, err := NewListOpenDisputes(repo).Run(context.Background(), RoleCustomer); !errors.Is(err, domain.ErrNotOperator) {
		t.Fatalf("customer err = %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository called %d times", repo.calls)
	}
	list, err := NewListOpenDisputes(repo).Run(context.Background(), RoleAdmin)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v err = %v", list, err)
	}
}
