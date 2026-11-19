package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// reconcileRepo adds the unrecorded-incident listing on top of the recording fake. failOrder makes the
// recording of one order fail, to prove the others are still recovered.
type reconcileRepo struct {
	*surchargeRepoFake
	unrecorded []domain.UnrecordedIncident
	listErr    error
	failOrder  string
	listLimit  int
}

func (r *reconcileRepo) ListUnrecordedIncidents(_ context.Context, limit int) ([]domain.UnrecordedIncident, error) {
	r.listLimit = limit
	return r.unrecorded, r.listErr
}

func (r *reconcileRepo) RecordSurchargeIncident(ctx context.Context, s domain.Surcharge) (domain.Surcharge, bool, error) {
	if s.OrderID == r.failOrder {
		return domain.Surcharge{}, false, errors.New("db down")
	}
	return r.surchargeRepoFake.RecordSurchargeIncident(ctx, s)
}

var closedAt = time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)

func newReconcile(repo *reconcileRepo) *ReconcileIncidents {
	return NewReconcileIncidents(repo, NewIncidentRecorder(repo, nil))
}

func TestReconcileRecoversTheLostIncidentWithItsOriginalDateAndThe15PercentPrice(t *testing.T) {
	repo := &reconcileRepo{
		surchargeRepoFake: &surchargeRepoFake{base: decimal.RequireFromString("20.00"), baseFound: true},
		unrecorded:        []domain.UnrecordedIncident{{OrderID: "o-1", CustomerID: "c-1", Reason: "CLIENT_REFUSED", RecordedAt: closedAt}},
	}

	n, err := newReconcile(repo).Run(context.Background())

	if err != nil || n != 1 {
		t.Fatalf("recovered = %d, err = %v", n, err)
	}
	got := repo.recorded[0]
	if got.OrderID != "o-1" || got.CustomerID != "c-1" || got.Reason != "CLIENT_REFUSED" || got.Status != domain.SurchargePending {
		t.Fatalf("incident = %+v", got)
	}
	if priced(t, got) != "3.00" {
		t.Fatalf("amount = %s, want 3.00", priced(t, got))
	}
	if !got.CreatedAt.Equal(closedAt) {
		t.Fatalf("created at = %s, want the original close time %s", got.CreatedAt, closedAt)
	}
}

func TestReconcileRecoversAWrongAddressAsPendingVerification(t *testing.T) {
	repo := &reconcileRepo{
		surchargeRepoFake: &surchargeRepoFake{base: decimal.RequireFromString("10.00"), baseFound: true},
		unrecorded:        []domain.UnrecordedIncident{{OrderID: "o-1", CustomerID: "c-1", Reason: "WRONG_ADDRESS", RecordedAt: closedAt}},
	}

	if _, err := newReconcile(repo).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := repo.recorded[0]; got.Status != domain.SurchargePendingVerification || priced(t, got) != "1.50" {
		t.Fatalf("incident = %+v", got)
	}
}

// Only newly created incidents count as recovered: one that already existed is not counted twice.
func TestReconcileCountsOnlyWhatItCreated(t *testing.T) {
	repo := &reconcileRepo{
		surchargeRepoFake: &surchargeRepoFake{base: decimal.NewFromInt(10), baseFound: true, existing: map[string]bool{"o-1": true}},
		unrecorded: []domain.UnrecordedIncident{
			{OrderID: "o-1", CustomerID: "c-1", Reason: "CLIENT_ABSENT", RecordedAt: closedAt},
			{OrderID: "o-2", CustomerID: "c-1", Reason: "CLIENT_ABSENT", RecordedAt: closedAt},
		},
	}

	n, err := newReconcile(repo).Run(context.Background())

	if err != nil || n != 1 {
		t.Fatalf("recovered = %d, err = %v, want 1", n, err)
	}
}

func TestReconcileKeepsGoingWhenOneIncidentFails(t *testing.T) {
	repo := &reconcileRepo{
		surchargeRepoFake: &surchargeRepoFake{base: decimal.NewFromInt(10), baseFound: true},
		failOrder:         "o-1",
		unrecorded: []domain.UnrecordedIncident{
			{OrderID: "o-1", CustomerID: "c-1", Reason: "CLIENT_REFUSED", RecordedAt: closedAt},
			{OrderID: "o-2", CustomerID: "c-2", Reason: "CLIENT_REFUSED", RecordedAt: closedAt},
		},
	}

	n, err := newReconcile(repo).Run(context.Background())

	if err != nil || n != 1 {
		t.Fatalf("recovered = %d, err = %v, want the second one recovered and no error", n, err)
	}
	if len(repo.recorded) != 1 || repo.recorded[0].OrderID != "o-2" {
		t.Fatalf("recorded = %+v", repo.recorded)
	}
}

func TestReconcileWithNothingLostDoesNothing(t *testing.T) {
	repo := &reconcileRepo{surchargeRepoFake: &surchargeRepoFake{}}

	n, err := newReconcile(repo).Run(context.Background())

	if err != nil || n != 0 || len(repo.recorded) != 0 {
		t.Fatalf("recovered = %d, err = %v", n, err)
	}
}

func TestReconcileReturnsTheListingError(t *testing.T) {
	repo := &reconcileRepo{surchargeRepoFake: &surchargeRepoFake{}, listErr: errors.New("db down")}
	if _, err := newReconcile(repo).Run(context.Background()); err == nil {
		t.Fatal("want the listing error")
	}
}

func TestReconcileAsksForABoundedBatch(t *testing.T) {
	repo := &reconcileRepo{surchargeRepoFake: &surchargeRepoFake{}}
	if _, err := newReconcile(repo).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.listLimit <= 0 || repo.listLimit > 1000 {
		t.Fatalf("batch limit = %d, want a positive bound", repo.listLimit)
	}
}

// A recovered incident that completes the threshold marks the customer, like a live one does.
func TestReconcileEvaluatesReincidenceForARecoveredPendingIncident(t *testing.T) {
	base := &surchargeRepoFake{base: decimal.NewFromInt(10), baseFound: true}
	repo := &reconcileRepo{
		surchargeRepoFake: base,
		unrecorded:        []domain.UnrecordedIncident{{OrderID: "o-1", CustomerID: "c-1", Reason: "CLIENT_REFUSED", RecordedAt: closedAt}},
	}
	reinc := &reincidenceRepoFake{times: at(0, 1, 2)}

	_, err := NewReconcileIncidents(repo, NewIncidentRecorder(repo, NewReincidence(reinc))).Run(context.Background())

	if err != nil || len(reinc.marked) != 1 {
		t.Fatalf("err = %v, marked = %v", err, reinc.marked)
	}
}
