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

type verificationRepoFake struct {
	ports.SurchargeRepository
	stored domain.Surcharge
	calls  int
	listed []domain.Surcharge
}

func (f *verificationRepoFake) ResolveSurchargeVerification(_ context.Context, id string, decide func(domain.Surcharge) (domain.Surcharge, error)) (domain.Surcharge, error) {
	f.calls++
	if id != f.stored.ID {
		return domain.Surcharge{}, domain.ErrSurchargeNotFound
	}
	next, err := decide(f.stored)
	if err != nil {
		return domain.Surcharge{}, err
	}
	f.stored = next
	return next, nil
}

func (f *verificationRepoFake) ListSurchargesPendingVerification(context.Context) ([]domain.Surcharge, error) {
	f.calls++
	return f.listed, nil
}

func waitingSurcharge() domain.Surcharge {
	return domain.Surcharge{ID: uuid.NewString(), CustomerID: "c1", OrderID: "o1", Reason: "WRONG_ADDRESS", Status: domain.SurchargePendingVerification}
}

func TestResolveSurchargeVerificationByOperator(t *testing.T) {
	for _, role := range []string{RoleOperator, RoleAdmin} {
		repo := &verificationRepoFake{stored: waitingSurcharge()}
		uc := NewResolveSurchargeVerification(repo)
		uc.clock = func() time.Time { return time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC) }

		got, err := uc.Run(context.Background(), ResolveSurchargeCommand{Role: role, OperatorID: "op-1", SurchargeID: repo.stored.ID, Decision: "WAIVE", Reason: "La direccion era del negocio"})
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if got.Status != domain.SurchargeWaived || got.ResolvedBy != "op-1" {
			t.Fatalf("%s: got %+v", role, got)
		}
	}
}

func TestResolveSurchargeVerificationRejectsBeforeRepository(t *testing.T) {
	id := uuid.NewString()
	cases := []struct {
		name string
		cmd  ResolveSurchargeCommand
		want error
	}{
		{"customer", ResolveSurchargeCommand{Role: "CUSTOMER", OperatorID: "u", SurchargeID: id, Decision: "WAIVE", Reason: "motivo valido"}, domain.ErrNotOperator},
		{"driver", ResolveSurchargeCommand{Role: RoleDriver, OperatorID: "u", SurchargeID: id, Decision: "WAIVE", Reason: "motivo valido"}, domain.ErrNotOperator},
		{"bad id", ResolveSurchargeCommand{Role: RoleOperator, OperatorID: "u", SurchargeID: "nope", Decision: "WAIVE", Reason: "motivo valido"}, domain.ErrSurchargeNotFound},
		{"unknown decision", ResolveSurchargeCommand{Role: RoleOperator, OperatorID: "u", SurchargeID: id, Decision: "MAYBE", Reason: "motivo valido"}, domain.ErrUnknownSurchargeDecision},
		{"short reason", ResolveSurchargeCommand{Role: RoleOperator, OperatorID: "u", SurchargeID: id, Decision: "WAIVE", Reason: "no"}, domain.ErrReasonRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &verificationRepoFake{stored: domain.Surcharge{ID: id, Status: domain.SurchargePendingVerification}}
			_, err := NewResolveSurchargeVerification(repo).Run(context.Background(), tc.cmd)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if repo.calls != 0 {
				t.Fatalf("repository called %d times", repo.calls)
			}
		})
	}
}

func TestResolveSurchargeVerificationTwiceFails(t *testing.T) {
	repo := &verificationRepoFake{stored: waitingSurcharge()}
	uc := NewResolveSurchargeVerification(repo)
	cmd := ResolveSurchargeCommand{Role: RoleOperator, OperatorID: "op-1", SurchargeID: repo.stored.ID, Decision: "CONFIRM", Reason: "El cliente confirmo la direccion"}
	if _, err := uc.Run(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Run(context.Background(), cmd); !errors.Is(err, domain.ErrSurchargeNotPendingVerification) {
		t.Fatalf("second resolve err = %v", err)
	}
}

func TestListPendingVerificationIsOperatorOnly(t *testing.T) {
	repo := &verificationRepoFake{listed: []domain.Surcharge{waitingSurcharge()}}
	uc := NewResolveSurchargeVerification(repo)
	if _, err := uc.ListPending(context.Background(), "CUSTOMER"); !errors.Is(err, domain.ErrNotOperator) {
		t.Fatalf("err = %v", err)
	}
	got, err := uc.ListPending(context.Background(), RoleOperator)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
}
