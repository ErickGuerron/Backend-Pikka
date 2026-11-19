package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type reassignmentRepoFake struct {
	ports.ReassignmentRepository
	fee      *domain.ReassignmentFee
	customer string
	err      error
	calls    int
	charged  []domain.ReassignmentCharge
}

func (f *reassignmentRepoFake) SetReassignmentFee(_ context.Context, fee domain.ReassignmentFee) (domain.ReassignmentFee, error) {
	f.calls++
	f.fee = &fee
	return fee, f.err
}

func (f *reassignmentRepoFake) CurrentReassignmentFee(context.Context) (domain.ReassignmentFee, error) {
	f.calls++
	if f.fee == nil {
		return domain.ReassignmentFee{}, domain.ErrReassignmentFeeNotSet
	}
	return *f.fee, f.err
}

func (f *reassignmentRepoFake) ApplyReassignmentCharge(_ context.Context, _ string, build func(string, *domain.ReassignmentFee) (domain.ReassignmentCharge, error)) (domain.ReassignmentCharge, error) {
	f.calls++
	if f.err != nil {
		return domain.ReassignmentCharge{}, f.err
	}
	c, err := build(f.customer, f.fee)
	if err != nil {
		return domain.ReassignmentCharge{}, err
	}
	f.charged = append(f.charged, c)
	return c, nil
}

func TestSetReassignmentFeeOnlyAdminWithAuditedReason(t *testing.T) {
	repo := &reassignmentRepoFake{}
	cmd := SetReassignmentFeeCommand{Role: RoleAdmin, AdminID: "a-1", Amount: "2.50", Reason: "Sube la gasolina"}

	got, err := NewSetReassignmentFee(repo).Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount.StringFixed(2) != "2.50" || got.SetBy != "a-1" || got.Reason != "Sube la gasolina" {
		t.Fatalf("fee = %+v", got)
	}

	bad := map[string]struct {
		mutate func(*SetReassignmentFeeCommand)
		want   error
	}{
		"operator":     {func(c *SetReassignmentFeeCommand) { c.Role = RoleOperator }, domain.ErrNotAdmin},
		"zero":         {func(c *SetReassignmentFeeCommand) { c.Amount = "0" }, domain.ErrInvalidReassignmentFee},
		"negative":     {func(c *SetReassignmentFeeCommand) { c.Amount = "-1" }, domain.ErrInvalidReassignmentFee},
		"not number":   {func(c *SetReassignmentFeeCommand) { c.Amount = "x" }, domain.ErrInvalidReassignmentFee},
		"short reason": {func(c *SetReassignmentFeeCommand) { c.Reason = "ok" }, domain.ErrReasonRequired},
	}
	for name, tc := range bad {
		t.Run(name, func(t *testing.T) {
			r := &reassignmentRepoFake{}
			c := cmd
			tc.mutate(&c)
			if _, err := NewSetReassignmentFee(r).Run(context.Background(), c); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if r.calls != 0 {
				t.Fatalf("repository called %d times", r.calls)
			}
		})
	}
}

func TestGetReassignmentFeeStaffOnlyAndNeverInventsAnAmount(t *testing.T) {
	if _, err := NewGetReassignmentFee(&reassignmentRepoFake{}).Run(context.Background(), RoleCustomer); !errors.Is(err, domain.ErrNotOperator) {
		t.Fatalf("customer err = %v", err)
	}
	if _, err := NewGetReassignmentFee(&reassignmentRepoFake{}).Run(context.Background(), RoleOperator); !errors.Is(err, domain.ErrReassignmentFeeNotSet) {
		t.Fatalf("unset err = %v", err)
	}
}

func fee250() *domain.ReassignmentFee {
	return &domain.ReassignmentFee{Amount: decimal.RequireFromString("2.50"), SetBy: "a-1", Reason: "Sube la gasolina"}
}

func TestApplyReassignmentChargeUsesTheCurrentFee(t *testing.T) {
	repo := &reassignmentRepoFake{fee: fee250(), customer: uuid.NewString()}
	cmd := ApplyReassignmentChargeCommand{Role: RoleOperator, OperatorID: "op-1", OrderID: uuid.NewString(), Reason: "Pidió cambiar la entrega con el repartidor en camino"}

	got, err := NewApplyReassignmentCharge(repo).Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount.StringFixed(2) != "2.50" || got.AppliedBy != "op-1" || got.OrderID != cmd.OrderID || got.CustomerID != repo.customer {
		t.Fatalf("charge = %+v", got)
	}
}

func TestApplyReassignmentChargeRejectsBeforeCharging(t *testing.T) {
	good := ApplyReassignmentChargeCommand{Role: RoleOperator, OperatorID: "op-1", OrderID: uuid.NewString(), Reason: "Pidió cambiar la entrega con el repartidor en camino"}
	cases := map[string]struct {
		mutate func(*ApplyReassignmentChargeCommand)
		fee    *domain.ReassignmentFee
		want   error
		calls  int
	}{
		"customer":     {func(c *ApplyReassignmentChargeCommand) { c.Role = RoleCustomer }, fee250(), domain.ErrNotOperator, 0},
		"bad order id": {func(c *ApplyReassignmentChargeCommand) { c.OrderID = "x" }, fee250(), domain.ErrOrderNotFound, 0},
		"short reason": {func(c *ApplyReassignmentChargeCommand) { c.Reason = "ok" }, fee250(), domain.ErrReasonRequired, 0},
		"fee not set":  {func(*ApplyReassignmentChargeCommand) {}, nil, domain.ErrReassignmentFeeNotSet, 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &reassignmentRepoFake{fee: tc.fee, customer: uuid.NewString()}
			c := good
			tc.mutate(&c)
			if _, err := NewApplyReassignmentCharge(repo).Run(context.Background(), c); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != tc.calls || len(repo.charged) != 0 {
				t.Fatalf("calls = %d charged = %d", repo.calls, len(repo.charged))
			}
		})
	}
}

func TestApplyReassignmentChargePropagatesRepositoryDecisions(t *testing.T) {
	for _, want := range []error{domain.ErrOrderNotFound, domain.ErrReassignmentChargeExists} {
		repo := &reassignmentRepoFake{err: want}
		cmd := ApplyReassignmentChargeCommand{Role: RoleAdmin, OperatorID: "op-1", OrderID: uuid.NewString(), Reason: "Pidió cambiar la entrega con el repartidor en camino"}
		if _, err := NewApplyReassignmentCharge(repo).Run(context.Background(), cmd); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}
}
