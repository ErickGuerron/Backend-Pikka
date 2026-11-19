package domain

import (
	"errors"
	"time"
)

// DisputeWindow is how long a customer has to dispute a surcharge once it is owed (ADR 0007, section 6: "por ejemplo 2
// horas desde la notificacion"). The notice is created in the same transaction that makes the surcharge owed, so the
// window counts from the moment the payment clock starts.
const DisputeWindow = 2 * time.Hour

var (
	ErrDisputeWindowClosed    = errors.New("the dispute window for this surcharge is closed")
	ErrSurchargeNotDisputable = errors.New("only a surcharge that is owed and priced can be disputed")
	ErrDisputeExists          = errors.New("this surcharge already has a dispute")
	ErrDisputeNotFound        = errors.New("dispute not found")
	ErrDisputeAlreadyResolved = errors.New("the dispute is already resolved")
	ErrUnknownDisputeDecision = errors.New("dispute decision must be IN_FAVOR or AGAINST")
)

// DisputeStatus is where a dispute is. A resolved dispute carries its outcome as its status: IN_FAVOR waives the
// surcharge, AGAINST leaves it as it was.
type DisputeStatus string

const (
	DisputeOpen    DisputeStatus = "OPEN"
	DisputeInFavor DisputeStatus = "IN_FAVOR"
	DisputeAgainst DisputeStatus = "AGAINST"
)

// Dispute is a customer's objection to one surcharge. While it is open the overdue sweep does not block the account
// for that surcharge (ADR 0006, section 5). There is one dispute per surcharge.
type Dispute struct {
	ID          string
	SurchargeID string
	CustomerID  string
	Reason      string
	Status      DisputeStatus
	OpenedAt    time.Time

	// Audit of the operator's decision (empty while the dispute is open).
	ResolvedBy       string
	ResolutionReason string
	ResolvedAt       time.Time
}

// NewDispute opens a dispute over an owed, priced surcharge, inside the window and with the customer's reason
// (5 to 300 characters). Who owns the surcharge is checked by the repository under its lock.
func NewDispute(s Surcharge, reason string, at time.Time) (Dispute, error) {
	if s.Status != SurchargePending || !s.Amount.Valid {
		return Dispute{}, ErrSurchargeNotDisputable
	}
	if at.After(s.ClockStart().Add(DisputeWindow)) {
		return Dispute{}, ErrDisputeWindowClosed
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return Dispute{}, err
	}
	return Dispute{SurchargeID: s.ID, CustomerID: s.CustomerID, Reason: reason, Status: DisputeOpen, OpenedAt: at.UTC()}, nil
}

// Resolve closes an open dispute, once, with the deciding operator and an audited reason. A rejected call returns the
// dispute unchanged.
func (d Dispute) Resolve(decision DisputeStatus, operatorID, reason string, at time.Time) (Dispute, error) {
	if d.Status != DisputeOpen {
		return d, ErrDisputeAlreadyResolved
	}
	if decision != DisputeInFavor && decision != DisputeAgainst {
		return d, ErrUnknownDisputeDecision
	}
	if operatorID == "" {
		return d, ErrInvalidSurchargeIncident
	}
	reason, err := ReviewDecisionReason(reason)
	if err != nil {
		return d, err
	}
	d.Status, d.ResolvedBy, d.ResolutionReason, d.ResolvedAt = decision, operatorID, reason, at.UTC()
	return d, nil
}
