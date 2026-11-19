package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestNewStopBuildsTheWindowOnTheDeliveryDayInTheRecordingOffset(t *testing.T) {
	recordedAt := time.Date(2026, 10, 25, 10, 30, 0, 0, time.FixedZone("ECT", -5*3600))
	loc := domain.GeoPoint{Latitude: -0.18, Longitude: -78.48}

	stop, err := domain.NewStop(loc, "2026-10-25", "10:00", "12:00", recordedAt)

	require.NoError(t, err)
	assert.Equal(t, loc, stop.Location)
	assert.True(t, stop.Window.Start.Equal(time.Date(2026, 10, 25, 10, 0, 0, 0, recordedAt.Location())))
	assert.True(t, stop.Window.End.Equal(time.Date(2026, 10, 25, 12, 0, 0, 0, recordedAt.Location())))
}

func TestNewStopRejectsInvalidInput(t *testing.T) {
	at := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	loc := domain.GeoPoint{Latitude: -0.18, Longitude: -78.48}
	cases := map[string][4]string{
		"bad date":        {"25/10/2026", "10:00", "12:00", ""},
		"bad start clock": {"2026-10-25", "9:00", "12:00", ""},
		"reversed":        {"2026-10-25", "12:00", "10:00", ""},
		"equal":           {"2026-10-25", "10:00", "10:00", ""},
	}
	for name, c := range cases {
		_, err := domain.NewStop(loc, c[0], c[1], c[2], at)
		assert.ErrorIs(t, err, domain.ErrInvalidWindow, name)
	}
}
