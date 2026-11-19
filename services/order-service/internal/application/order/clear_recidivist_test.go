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

type clearRepoFake struct {
	ports.RecidivismRepository
	markedAt time.Time
	err      error
	calls    int
	cleared  []domain.RecidivistClear
}

func (f *clearRepoFake) ClearRecidivistMark(_ context.Context, _ string, build func(time.Time) (domain.RecidivistClear, error)) (domain.RecidivistClear, error) {
	f.calls++
	if f.err != nil {
		return domain.RecidivistClear{}, f.err
	}
	c, err := build(f.markedAt)
	if err != nil {
		return domain.RecidivistClear{}, err
	}
	f.cleared = append(f.cleared, c)
	return c, nil
}

func validClearCommand() ClearRecidivistMarkCommand {
	return ClearRecidivistMarkCommand{Role: RoleAdmin, AdminID: "admin-1", CustomerID: uuid.NewString(), Reason: "Se acordó con el cliente y pagó todo"}
}

func TestClearRecidivistMarkByAdmin(t *testing.T) {
	repo := &clearRepoFake{markedAt: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	cmd := validClearCommand()

	got, err := NewClearRecidivistMark(repo).Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.CustomerID != cmd.CustomerID || got.ClearedBy != "admin-1" || got.Reason != cmd.Reason || !got.RecidivistAt.Equal(repo.markedAt) {
		t.Fatalf("clear = %+v", got)
	}
}

func TestClearRecidivistMarkRejectsBeforeTheRepository(t *testing.T) {
	cases := map[string]struct {
		mutate func(*ClearRecidivistMarkCommand)
		want   error
	}{
		"operator is not enough":    {func(c *ClearRecidivistMarkCommand) { c.Role = RoleOperator }, domain.ErrNotAdmin},
		"customer is not enough":    {func(c *ClearRecidivistMarkCommand) { c.Role = RoleCustomer }, domain.ErrNotAdmin},
		"customer id is not a uuid": {func(c *ClearRecidivistMarkCommand) { c.CustomerID = "x" }, domain.ErrCustomerNotFound},
		"short reason":              {func(c *ClearRecidivistMarkCommand) { c.Reason = "ok" }, domain.ErrReasonRequired},
		"missing admin":             {func(c *ClearRecidivistMarkCommand) { c.AdminID = "" }, domain.ErrInvalidRecidivistClear},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &clearRepoFake{}
			cmd := validClearCommand()
			tc.mutate(&cmd)

			_, err := NewClearRecidivistMark(repo).Run(context.Background(), cmd)

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != 0 {
				t.Fatalf("repository called %d times", repo.calls)
			}
		})
	}
}

func TestClearRecidivistMarkPropagatesRepositoryDecisions(t *testing.T) {
	for _, want := range []error{domain.ErrCustomerNotRecidivist, domain.ErrCustomerNotFound} {
		_, err := NewClearRecidivistMark(&clearRepoFake{err: want}).Run(context.Background(), validClearCommand())
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}
}
