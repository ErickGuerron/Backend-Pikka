// Package domain contains the driver position model of the Tracking Service.
//
// A position is personal location data. It is held only as the latest fix per driver and never
// logged or returned to customers by this service.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

const (
	// FreshWindow is how old a fix may be and still be used for an estimate.
	FreshWindow = 120 * time.Second
	// MaxClockSkew is how far ahead of the server clock a fix timestamp may be.
	MaxClockSkew = 30 * time.Second
)

var (
	ErrInvalidPosition = errors.New("driver position is invalid")
	ErrFutureTimestamp = errors.New("driver position timestamp is ahead of the server clock")
)

// Position is the last known location of a driver.
type Position struct {
	DriverID   string
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

// NewPosition validates a GPS fix. now is the server clock, injected so the rules are testable.
func NewPosition(driverID string, latitude, longitude float64, recordedAt, now time.Time) (Position, error) {
	if strings.TrimSpace(driverID) == "" {
		return Position{}, ErrInvalidPosition
	}
	if math.IsNaN(latitude) || math.IsNaN(longitude) || math.IsInf(latitude, 0) || math.IsInf(longitude, 0) {
		return Position{}, ErrInvalidPosition
	}
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return Position{}, ErrInvalidPosition
	}
	if recordedAt.IsZero() {
		return Position{}, ErrInvalidPosition
	}
	if recordedAt.Sub(now) > MaxClockSkew {
		return Position{}, ErrFutureTimestamp
	}
	return Position{DriverID: driverID, Latitude: latitude, Longitude: longitude, RecordedAt: recordedAt}, nil
}

// IsFresh reports whether the fix is recent enough to base an estimate on.
func (p Position) IsFresh(now time.Time) bool {
	return now.Sub(p.RecordedAt) <= FreshWindow
}

// Supersedes reports whether p should replace prev. Only a strictly newer fix does, so late or
// duplicated packets cannot move the driver backwards.
func (p Position) Supersedes(prev Position) bool {
	return p.RecordedAt.After(prev.RecordedAt)
}
