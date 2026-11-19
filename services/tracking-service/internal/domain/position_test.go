package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func TestNewPositionAcceptsValidFix(t *testing.T) {
	got, err := NewPosition("driver-1", -0.18, -78.48, now, now)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	if got.DriverID != "driver-1" || got.Latitude != -0.18 || got.Longitude != -78.48 || !got.RecordedAt.Equal(now) {
		t.Fatalf("position = %+v", got)
	}
}

func TestNewPositionRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name       string
		driverID   string
		lat, lon   float64
		recordedAt time.Time
	}{
		{"blank driver", "  ", 0, 0, now},
		{"latitude above range", "d", 91, 0, now},
		{"longitude below range", "d", 0, -181, now},
		{"latitude NaN", "d", math.NaN(), 0, now},
		{"longitude infinite", "d", 0, math.Inf(1), now},
		{"zero timestamp", "d", 0, 0, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewPosition(tc.driverID, tc.lat, tc.lon, tc.recordedAt, now); !errors.Is(err, ErrInvalidPosition) {
				t.Fatalf("err = %v, want ErrInvalidPosition", err)
			}
		})
	}
}

// A timestamp more than 30 s ahead of the server clock is rejected: it would otherwise look fresh forever.
func TestNewPositionRejectsFutureTimestamp(t *testing.T) {
	if _, err := NewPosition("d", 0, 0, now.Add(31*time.Second), now); !errors.Is(err, ErrFutureTimestamp) {
		t.Fatalf("err = %v, want ErrFutureTimestamp", err)
	}
	if _, err := NewPosition("d", 0, 0, now.Add(30*time.Second), now); err != nil {
		t.Fatalf("30 s of skew must be accepted: %v", err)
	}
}

func TestPositionFreshnessWindowIs120Seconds(t *testing.T) {
	p, _ := NewPosition("d", 0, 0, now, now)
	if !p.IsFresh(now.Add(120 * time.Second)) {
		t.Fatal("position at exactly 120 s must still be fresh")
	}
	if p.IsFresh(now.Add(121 * time.Second)) {
		t.Fatal("position older than 120 s must be stale")
	}
}

// Ordering: a fix only supersedes the stored one when it is strictly newer.
func TestSupersedesRequiresStrictlyNewerTimestamp(t *testing.T) {
	older, _ := NewPosition("d", 0, 0, now, now)
	newer, _ := NewPosition("d", 1, 1, now.Add(5*time.Second), now.Add(5*time.Second))
	same, _ := NewPosition("d", 2, 2, now, now)

	if !newer.Supersedes(older) {
		t.Fatal("newer fix must supersede older")
	}
	if older.Supersedes(newer) {
		t.Fatal("out-of-order fix must not supersede the stored one")
	}
	if same.Supersedes(older) {
		t.Fatal("equal timestamp must not supersede")
	}
}
