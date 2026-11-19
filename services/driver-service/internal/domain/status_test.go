package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

func TestParseStatus(t *testing.T) {
	for _, s := range []domain.Status{domain.StatusAvailable, domain.StatusAssigned, domain.StatusOnRoute, domain.StatusOffline} {
		got, err := domain.ParseStatus(string(s))
		require.NoError(t, err)
		assert.Equal(t, s, got)
	}

	for _, raw := range []string{"", "available", "BUSY", " AVAILABLE"} {
		_, err := domain.ParseStatus(raw)
		assert.ErrorIs(t, err, domain.ErrInvalidStatus, raw)
	}
}

func TestStatusValid(t *testing.T) {
	assert.True(t, domain.StatusOnRoute.Valid())
	assert.False(t, domain.Status("").Valid())
	assert.False(t, domain.Status("UNKNOWN").Valid())
}

func TestStatusCanTransitionTo(t *testing.T) {
	A, S, R, O := domain.StatusAvailable, domain.StatusAssigned, domain.StatusOnRoute, domain.StatusOffline
	tests := []struct {
		from, to domain.Status
		want     bool
	}{
		{A, A, false}, {A, S, true}, {A, R, false}, {A, O, true},
		{S, A, true}, {S, S, false}, {S, R, true}, {S, O, true},
		{R, A, true}, {R, S, false}, {R, R, false}, {R, O, false},
		{O, A, true}, {O, S, false}, {O, R, false}, {O, O, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+" a "+string(tt.to), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.from.CanTransitionTo(tt.to))
		})
	}
}

func TestStatusCanTransitionToRechazaEstadosInvalidos(t *testing.T) {
	assert.False(t, domain.Status("UNKNOWN").CanTransitionTo(domain.StatusAvailable))
	assert.False(t, domain.StatusAvailable.CanTransitionTo(domain.Status("UNKNOWN")))
}
