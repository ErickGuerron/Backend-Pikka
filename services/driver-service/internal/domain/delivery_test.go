package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

var (
	quito      = domain.GeoPoint{Latitude: -0.1807, Longitude: -78.4678}
	nearQuito  = domain.GeoPoint{Latitude: -0.1810, Longitude: -78.4680} // ~40 m
	farFromQto = domain.GeoPoint{Latitude: -0.2500, Longitude: -78.5000} // varios km
)

func TestNewDeliveryRecordDelivered(t *testing.T) {
	rec, err := domain.NewDeliveryRecord("stop-1", domain.OutcomeDelivered, "", t0, quito, "")

	require.NoError(t, err)
	assert.Equal(t, domain.OutcomeDelivered, rec.Outcome)
	assert.Empty(t, rec.Reason)
}

func TestNewDeliveryRecordValidaciones(t *testing.T) {
	bad := domain.GeoPoint{Latitude: 91, Longitude: 0}
	nan := domain.GeoPoint{Latitude: math.NaN(), Longitude: 0}
	tests := []struct {
		name    string
		stopID  string
		outcome domain.Outcome
		reason  domain.NotDeliveredReason
		loc     domain.GeoPoint
		want    error
	}{
		{name: "DELIVERED con motivo", stopID: "s", outcome: domain.OutcomeDelivered, reason: domain.ReasonClientAbsent, loc: quito, want: domain.ErrReasonNotAllowed},
		{name: "NOT_DELIVERED sin motivo", stopID: "s", outcome: domain.OutcomeNotDelivered, loc: quito, want: domain.ErrReasonRequired},
		{name: "NOT_DELIVERED motivo fuera de lista", stopID: "s", outcome: domain.OutcomeNotDelivered, reason: "OTRO", loc: quito, want: domain.ErrReasonRequired},
		{name: "resultado desconocido", stopID: "s", outcome: "PENDING", loc: quito, want: domain.ErrInvalidOutcome},
		{name: "parada vacía", stopID: "", outcome: domain.OutcomeDelivered, loc: quito, want: domain.ErrInvalidStopID},
		{name: "latitud fuera de rango", stopID: "s", outcome: domain.OutcomeDelivered, loc: bad, want: domain.ErrInvalidCoordinates},
		{name: "coordenada NaN", stopID: "s", outcome: domain.OutcomeDelivered, loc: nan, want: domain.ErrInvalidCoordinates},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewDeliveryRecord(tt.stopID, tt.outcome, tt.reason, t0, tt.loc, "")
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestImputableOf(t *testing.T) {
	client := []domain.NotDeliveredReason{domain.ReasonClientRefused, domain.ReasonClientAbsent, domain.ReasonOutOfWindow}
	for _, r := range client {
		got, ok := domain.ImputableOf(r)
		assert.True(t, ok)
		assert.Equal(t, domain.ImputableClient, got, string(r))
	}
	business := []domain.NotDeliveredReason{domain.ReasonProductDamage, domain.ReasonWrongAddress, domain.ReasonDuplicateOrder}
	for _, r := range business {
		got, ok := domain.ImputableOf(r)
		assert.True(t, ok)
		assert.Equal(t, domain.ImputableBusiness, got, string(r))
	}
	_, ok := domain.ImputableOf("OTRO")
	assert.False(t, ok)
}

func TestDistanceMeters(t *testing.T) {
	assert.InDelta(t, 0, domain.DistanceMeters(quito, quito), 0.001)

	// Un grado de longitud sobre el ecuador mide ~111.195 km.
	d := domain.DistanceMeters(domain.GeoPoint{}, domain.GeoPoint{Longitude: 1})
	assert.InDelta(t, 111195, d, 200)

	assert.InDelta(t, 40, domain.DistanceMeters(quito, nearQuito), 15)
}

func TestVerifyArrival(t *testing.T) {
	window := domain.ArrivalWindow{Start: t0, End: t0.Add(3 * time.Hour)}
	stop := domain.Stop{Location: quito, Window: window}
	rec := func(at time.Time, loc domain.GeoPoint) domain.DeliveryRecord {
		r, err := domain.NewDeliveryRecord("stop-1", domain.OutcomeDelivered, "", at, loc, "")
		require.NoError(t, err)
		return r
	}

	tests := []struct {
		name       string
		at         time.Time
		loc        domain.GeoPoint
		wantValid  bool
		wantRadius bool
		wantWindow bool
	}{
		{name: "dentro de radio y ventana", at: t0.Add(time.Hour), loc: nearQuito, wantValid: true, wantRadius: true, wantWindow: true},
		{name: "fuera de radio", at: t0.Add(time.Hour), loc: farFromQto, wantRadius: false, wantWindow: true},
		{name: "antes de la ventana", at: t0.Add(-time.Minute), loc: nearQuito, wantRadius: true, wantWindow: false},
		{name: "inicio de ventana incluido", at: t0, loc: nearQuito, wantValid: true, wantRadius: true, wantWindow: true},
		{name: "fin de ventana excluido", at: window.End, loc: nearQuito, wantRadius: true, wantWindow: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.VerifyArrival(stop, rec(tt.at, tt.loc), 100)

			require.NoError(t, err)
			assert.Equal(t, tt.wantRadius, got.WithinRadius)
			assert.Equal(t, tt.wantWindow, got.WithinWindow)
			assert.Equal(t, tt.wantValid, got.Valid())
		})
	}
}

func TestVerifyArrivalValidaParametros(t *testing.T) {
	stop := domain.Stop{Location: quito, Window: domain.ArrivalWindow{Start: t0, End: t0.Add(time.Hour)}}
	rec, err := domain.NewDeliveryRecord("stop-1", domain.OutcomeDelivered, "", t0, quito, "")
	require.NoError(t, err)

	_, err = domain.VerifyArrival(stop, rec, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidRadius)

	badWindow := domain.Stop{Location: quito, Window: domain.ArrivalWindow{Start: t0, End: t0}}
	_, err = domain.VerifyArrival(badWindow, rec, 100)
	assert.ErrorIs(t, err, domain.ErrInvalidWindow)
}
