package domain_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

func TestValidateSettings_ValidNormalizes(t *testing.T) {
	in := validSettings()
	in.ServiceDays = []string{"sat", " mon ", "FRI"} // desordenados, con mayúsculas y espacios
	in.Priority = "high"
	in.Timezone = ""
	in.DepotName = "  Hub Norte  "

	out, err := domain.ValidateSettings(in)

	require.NoError(t, err)
	assert.Equal(t, []string{"MON", "FRI", "SAT"}, out.ServiceDays)
	assert.Equal(t, domain.PriorityHigh, out.Priority)
	assert.Equal(t, "America/Guayaquil", out.Timezone, "una zona horaria vacía usa la de Ecuador")
	assert.Equal(t, "Hub Norte", out.DepotName)
}

func TestValidateSettings_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.Settings)
	}{
		{"description too long", func(s *domain.Settings) { s.Description = string(make([]byte, 501)) }},
		{"empty depot name", func(s *domain.Settings) { s.DepotName = "   " }},
		{"depot latitude out of range", func(s *domain.Settings) { s.DepotLatitude = 91 }},
		{"depot longitude NaN", func(s *domain.Settings) { s.DepotLongitude = math.NaN() }},
		{"zero orders per route", func(s *domain.Settings) { s.MaxOrdersPerRoute = 0 }},
		{"zero route capacity", func(s *domain.Settings) { s.RouteCapacityKg = 0 }},
		{"negative route capacity", func(s *domain.Settings) { s.RouteCapacityKg = -1 }},
		{"infinite route capacity", func(s *domain.Settings) { s.RouteCapacityKg = math.Inf(1) }},
		{"unknown timezone", func(s *domain.Settings) { s.Timezone = "Mars/Olympus" }},
		{"no service days", func(s *domain.Settings) { s.ServiceDays = nil }},
		{"repeated service day", func(s *domain.Settings) { s.ServiceDays = []string{"MON", "mon"} }},
		{"unknown service day", func(s *domain.Settings) { s.ServiceDays = []string{"LUNES"} }},
		{"window start not HH:MM", func(s *domain.Settings) { s.DeliveryWindowFrom = "8am" }},
		{"window start after end", func(s *domain.Settings) { s.DeliveryWindowFrom = "21:00" }},
		{"window equal start and end", func(s *domain.Settings) { s.DeliveryWindowTo = s.DeliveryWindowFrom }},
		{"cutoff hour out of range", func(s *domain.Settings) { s.CutoffTime = "24:00" }},
		{"dispatch not HH:MM", func(s *domain.Settings) { s.DispatchTime = "15.30" }},
		{"unknown priority", func(s *domain.Settings) { s.Priority = "URGENT" }},
		{"zero daily limit", func(s *domain.Settings) { s.DailyOrderLimit = 0 }},
		{"daily limit above warehouse capacity", func(s *domain.Settings) { s.DailyOrderLimit = 10001 }},
		{"orders per route above a van's load", func(s *domain.Settings) { s.MaxOrdersPerRoute = 51 }},
		{"route capacity above one ton", func(s *domain.Settings) { s.RouteCapacityKg = 1001 }},
		{"depot name with digits", func(s *domain.Settings) { s.DepotName = "Bodega 2" }},
		{"depot name too short", func(s *domain.Settings) { s.DepotName = "Hu" }},
		{"depot outside Ecuador", func(s *domain.Settings) { s.DepotLatitude = 40.7; s.DepotLongitude = -74.0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			tt.mutate(&s)
			_, err := domain.ValidateSettings(s)
			require.ErrorIs(t, err, domain.ErrInvalidSettings)
		})
	}
}

func TestValidateSettings_EmptyPriorityDefaultsToNormal(t *testing.T) {
	s := validSettings()
	s.Priority = ""

	out, err := domain.ValidateSettings(s)

	require.NoError(t, err)
	assert.Equal(t, domain.PriorityNormal, out.Priority)
}

func TestNewZoneWithHoles_RejectsInvalidSettings(t *testing.T) {
	s := validSettings()
	s.DailyOrderLimit = 0

	_, err := domain.NewZoneWithHoles("NORTE", "Zona Norte", square(t), nil, s)

	require.ErrorIs(t, err, domain.ErrInvalidSettings)
}

func TestNewZoneWithHoles_StartsActiveVersionOne(t *testing.T) {
	zone, err := newZone("NORTE", "Zona Norte", square(t))

	require.NoError(t, err)
	assert.Equal(t, domain.StatusActive, zone.Status)
	assert.Equal(t, 1, zone.Version)
	assert.Equal(t, "Hub Norte", zone.Settings.DepotName)
}
