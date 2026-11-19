package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type recidivistsRepoFake struct {
	ports.RecidivismRepository
	list  []domain.Recidivist
	err   error
	calls int
}

func (f *recidivistsRepoFake) ListRecidivists(context.Context) ([]domain.Recidivist, error) {
	f.calls++
	return f.list, f.err
}

func TestListRecidivistsServesOperatorAndAdmin(t *testing.T) {
	want := []domain.Recidivist{{CustomerID: "c-1", FullName: "Ana Pérez", PendingSurcharge: decimal.RequireFromString("4.50"), RecidivistAt: time.Now()}}
	for _, role := range []string{RoleOperator, RoleAdmin} {
		repo := &recidivistsRepoFake{list: want}

		got, err := NewListRecidivists(repo).Run(context.Background(), role)

		if err != nil || len(got) != 1 || got[0].CustomerID != "c-1" {
			t.Fatalf("%s: got %v, %v", role, got, err)
		}
	}
}

func TestListRecidivistsRefusesEveryOtherRoleBeforeTheRepository(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER", ""} {
		repo := &recidivistsRepoFake{}

		_, err := NewListRecidivists(repo).Run(context.Background(), role)

		if !errors.Is(err, domain.ErrNotOperator) || repo.calls != 0 {
			t.Fatalf("%q: err = %v, repository calls = %d", role, err, repo.calls)
		}
	}
}

func TestListRecidivistsPassesTheRepositoryErrorThrough(t *testing.T) {
	boom := errors.New("db down")
	_, err := NewListRecidivists(&recidivistsRepoFake{err: boom}).Run(context.Background(), RoleOperator)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
