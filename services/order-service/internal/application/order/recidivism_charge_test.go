package order

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type recidivismRepoFake struct {
	ports.RecidivismRepository
	markedAt time.Time
	err      error
	calls    int
	applied  []domain.RecidivismCharge
}

func (f *recidivismRepoFake) ApplyRecidivismCharge(_ context.Context, customerID string, build func(time.Time) (domain.RecidivismCharge, error)) (domain.RecidivismCharge, error) {
	f.calls++
	if f.err != nil {
		return domain.RecidivismCharge{}, f.err
	}
	charge, err := build(f.markedAt)
	if err != nil {
		return domain.RecidivismCharge{}, err
	}
	f.applied = append(f.applied, charge)
	return charge, nil
}

func validRecidivismCommand() ApplyRecidivismChargeCommand {
	return ApplyRecidivismChargeCommand{Role: RoleOperator, OperatorID: "op-1", CustomerID: uuid.NewString(), Amount: "12.50", Reason: "Tres no entregas en dos semanas"}
}

func TestApplyRecidivismChargeByOperatorAndAdmin(t *testing.T) {
	for _, role := range []string{RoleOperator, RoleAdmin} {
		repo := &recidivismRepoFake{markedAt: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
		cmd := validRecidivismCommand()
		cmd.Role = role

		got, err := NewApplyRecidivismCharge(repo, testRecidivismCap).Run(context.Background(), cmd)

		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if got.AppliedBy != "op-1" || got.Amount.StringFixed(2) != "12.50" || !got.RecidivistAt.Equal(repo.markedAt) {
			t.Fatalf("%s: charge = %+v", role, got)
		}
	}
}

func TestApplyRecidivismChargeRejectsBeforeRepository(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ApplyRecidivismChargeCommand)
		want   error
	}{
		{"customer role", func(c *ApplyRecidivismChargeCommand) { c.Role = "CUSTOMER" }, domain.ErrNotOperator},
		{"driver role", func(c *ApplyRecidivismChargeCommand) { c.Role = RoleDriver }, domain.ErrNotOperator},
		{"bad customer id", func(c *ApplyRecidivismChargeCommand) { c.CustomerID = "nope" }, domain.ErrCustomerNotFound},
		{"amount not a number", func(c *ApplyRecidivismChargeCommand) { c.Amount = "doce" }, domain.ErrInvalidRecidivismCharge},
		{"empty amount", func(c *ApplyRecidivismChargeCommand) { c.Amount = "" }, domain.ErrInvalidRecidivismCharge},
		{"zero amount", func(c *ApplyRecidivismChargeCommand) { c.Amount = "0" }, domain.ErrInvalidRecidivismCharge},
		{"short reason", func(c *ApplyRecidivismChargeCommand) { c.Reason = "no" }, domain.ErrReasonRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &recidivismRepoFake{}
			cmd := validRecidivismCommand()
			tc.mutate(&cmd)

			_, err := NewApplyRecidivismCharge(repo, testRecidivismCap).Run(context.Background(), cmd)

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != 0 {
				t.Fatalf("repository called %d times", repo.calls)
			}
		})
	}
}

func TestApplyRecidivismChargePropagatesRepositoryDecisions(t *testing.T) {
	for _, want := range []error{domain.ErrCustomerNotRecidivist, domain.ErrRecidivismChargeExists, domain.ErrCustomerNotFound} {
		repo := &recidivismRepoFake{err: want}
		_, err := NewApplyRecidivismCharge(repo, testRecidivismCap).Run(context.Background(), validRecidivismCommand())
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}
}

var testRecidivismCap = decimal.NewFromInt(50)

func TestApplyRecidivismChargeRespectsTheCap(t *testing.T) {
	t.Run("the cap itself is allowed", func(t *testing.T) {
		repo := &recidivismRepoFake{markedAt: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
		cmd := validRecidivismCommand()
		cmd.Amount = "50.00"
		if _, err := NewApplyRecidivismCharge(repo, testRecidivismCap).Run(context.Background(), cmd); err != nil {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("above the cap is rejected before the repository, naming the cap", func(t *testing.T) {
		repo := &recidivismRepoFake{}
		cmd := validRecidivismCommand()
		cmd.Amount = "50.01"
		_, err := NewApplyRecidivismCharge(repo, testRecidivismCap).Run(context.Background(), cmd)
		if !errors.Is(err, domain.ErrRecidivismChargeTooHigh) {
			t.Fatalf("err = %v, want ErrRecidivismChargeTooHigh", err)
		}
		if !strings.Contains(err.Error(), "50.00") {
			t.Fatalf("message %q does not name the cap", err.Error())
		}
		if repo.calls != 0 {
			t.Fatalf("repository called %d times", repo.calls)
		}
	})
	t.Run("a typo with extra zeros is caught", func(t *testing.T) {
		repo := &recidivismRepoFake{}
		cmd := validRecidivismCommand()
		cmd.Amount = "1250.00"
		if _, err := NewApplyRecidivismCharge(repo, testRecidivismCap).Run(context.Background(), cmd); !errors.Is(err, domain.ErrRecidivismChargeTooHigh) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a cap that is not positive rejects everything instead of disabling the limit", func(t *testing.T) {
		repo := &recidivismRepoFake{}
		if _, err := NewApplyRecidivismCharge(repo, decimal.Zero).Run(context.Background(), validRecidivismCommand()); !errors.Is(err, domain.ErrRecidivismChargeTooHigh) {
			t.Fatalf("err = %v", err)
		}
	})
}
