package domain_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

var allStatuses = []domain.Status{domain.StatusAvailable, domain.StatusAssigned, domain.StatusOnRoute, domain.StatusOffline}

var (
	t0 = time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Hour)
)

func validInput() domain.NewDriverInput {
	return domain.NewDriverInput{UserID: "user-1", FullName: " Carlos Pérez ", Phone: "+593 99-123 4567"}
}

func newDriver(t *testing.T, status domain.Status) *domain.Driver {
	t.Helper()
	d, err := domain.NewDriver("driver-1", validInput(), t0)
	require.NoError(t, err)
	d.Status = status
	if status == domain.StatusAssigned || status == domain.StatusOnRoute {
		d.RouteID = "route-1"
	}
	return d
}

func TestNewDriverInputValidate(t *testing.T) {
	got, err := validInput().Validate()
	require.NoError(t, err)
	assert.Equal(t, "Carlos Pérez", got.FullName)
	assert.Equal(t, "+593991234567", got.Phone)
	assert.Equal(t, "user-1", got.UserID)

	tests := []struct {
		name   string
		mutate func(*domain.NewDriverInput)
		want   error
	}{
		{name: "nombre vacío", mutate: func(in *domain.NewDriverInput) { in.FullName = "   " }, want: domain.ErrInvalidFullName},
		{name: "nombre demasiado largo", mutate: func(in *domain.NewDriverInput) { in.FullName = strings.Repeat("á", domain.MaxFullNameRunes+1) }, want: domain.ErrInvalidFullName},
		{name: "teléfono vacío", mutate: func(in *domain.NewDriverInput) { in.Phone = "" }, want: domain.ErrInvalidPhone},
		{name: "teléfono con letras", mutate: func(in *domain.NewDriverInput) { in.Phone = "099abc4567" }, want: domain.ErrInvalidPhone},
		{name: "teléfono demasiado corto", mutate: func(in *domain.NewDriverInput) { in.Phone = "123456" }, want: domain.ErrInvalidPhone},
		{name: "teléfono demasiado largo", mutate: func(in *domain.NewDriverInput) { in.Phone = "+1234567890123456" }, want: domain.ErrInvalidPhone},
		{name: "más sin ser prefijo", mutate: func(in *domain.NewDriverInput) { in.Phone = "5939+9123456" }, want: domain.ErrInvalidPhone},
		{name: "usuario vacío", mutate: func(in *domain.NewDriverInput) { in.UserID = " " }, want: domain.ErrInvalidUserID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			_, err := in.Validate()
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestNewDriverInputValidateAceptaTelefonoSinPrefijo(t *testing.T) {
	in := validInput()
	in.Phone = "0991234567"
	got, err := in.Validate()
	require.NoError(t, err)
	assert.Equal(t, "0991234567", got.Phone)
}

func TestNewDriver(t *testing.T) {
	d, err := domain.NewDriver("driver-1", validInput(), t0)
	require.NoError(t, err)
	assert.Equal(t, "driver-1", d.ID)
	assert.Equal(t, "user-1", d.UserID)
	assert.Equal(t, "Carlos Pérez", d.FullName)
	assert.True(t, d.Active)
	assert.Equal(t, domain.StatusAvailable, d.Status)
	assert.Equal(t, 1, d.Version)
	assert.Equal(t, t0, d.CreatedAt)
	assert.Equal(t, t0, d.UpdatedAt)

	in := validInput()
	in.FullName = ""
	_, err = domain.NewDriver("driver-1", in, t0)
	assert.ErrorIs(t, err, domain.ErrInvalidFullName)
}

func TestNewDriverIDInvalido(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"id vacío", ""},
		{"id solo con espacios", "   "},
		{"id solo con tabulación y salto de línea", "\t\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := domain.NewDriver(tt.id, validInput(), t0)

			assert.ErrorIs(t, err, domain.ErrInvalidDriverID)
			assert.Nil(t, d)
		})
	}
}

func TestNewDriverGuardaElIDSinEspacios(t *testing.T) {
	d, err := domain.NewDriver("  driver-9 ", validInput(), t0)

	require.NoError(t, err)
	assert.Equal(t, "driver-9", d.ID)
}

func TestDriverTransiciones(t *testing.T) {
	A, S, R, O := domain.StatusAvailable, domain.StatusAssigned, domain.StatusOnRoute, domain.StatusOffline
	methods := []struct {
		name string
		call func(*domain.Driver, time.Time) error
		from []domain.Status
		to   domain.Status
	}{
		{"Release", releaseRoute1, []domain.Status{S}, A},
		{"StartRoute", startRoute1, []domain.Status{S}, R},
		{"CompleteRoute", completeRoute1, []domain.Status{R}, A},
		{"GoOffline", (*domain.Driver).GoOffline, []domain.Status{A, S}, O},
		{"GoOnline", (*domain.Driver).GoOnline, []domain.Status{O}, A},
	}
	for _, m := range methods {
		for _, from := range []domain.Status{A, S, R, O} {
			t.Run(m.name+" desde "+string(from), func(t *testing.T) {
				d := newDriver(t, from)
				version := d.Version
				err := m.call(d, t1)
				if contains(m.from, from) {
					require.NoError(t, err)
					assert.Equal(t, m.to, d.Status)
					assert.Equal(t, version+1, d.Version)
					assert.Equal(t, t1, d.UpdatedAt)
					return
				}
				assert.ErrorIs(t, err, domain.ErrInvalidTransition)
				assert.Equal(t, from, d.Status)
				assert.Equal(t, version, d.Version)
				assert.Equal(t, t0, d.UpdatedAt)
			})
		}
	}
}

func TestDriverReserve(t *testing.T) {
	d := newDriver(t, domain.StatusAvailable)

	require.NoError(t, d.Reserve("  route-1 ", t1))

	assert.Equal(t, domain.StatusAssigned, d.Status)
	assert.Equal(t, "route-1", d.RouteID)
	assert.Equal(t, 2, d.Version)
	assert.Equal(t, t1, d.UpdatedAt)
}

func TestDriverReserveRutaInvalida(t *testing.T) {
	d := newDriver(t, domain.StatusAvailable)

	err := d.Reserve("   ", t1)

	assert.ErrorIs(t, err, domain.ErrInvalidRouteID)
	assertUnchanged(t, d, domain.StatusAvailable, "", 1)
}

func TestDriverReserveInactivoSinReserva(t *testing.T) {
	for _, from := range []domain.Status{domain.StatusAvailable, domain.StatusOffline} {
		t.Run(string(from), func(t *testing.T) {
			d := newDriver(t, from)
			d.Active = false

			err := d.Reserve("route-1", t1)

			assert.ErrorIs(t, err, domain.ErrDriverInactive)
			assertUnchanged(t, d, from, "", 1)
		})
	}
}

func TestDriverReserveMismaRutaEsIdempotente(t *testing.T) {
	for _, active := range []bool{true, false} {
		for _, from := range []domain.Status{domain.StatusAssigned, domain.StatusOnRoute} {
			t.Run(fmt.Sprintf("%s activo=%t", from, active), func(t *testing.T) {
				d := newDriver(t, from)
				d.Active = active

				err := d.Reserve(" route-1", t1)

				require.NoError(t, err)
				assertUnchanged(t, d, from, "route-1", 1)
			})
		}
	}
}

func TestDriverReserveOtraRutaSeRechaza(t *testing.T) {
	for _, active := range []bool{true, false} {
		for _, from := range []domain.Status{domain.StatusAssigned, domain.StatusOnRoute} {
			t.Run(fmt.Sprintf("%s activo=%t", from, active), func(t *testing.T) {
				d := newDriver(t, from)
				d.Active = active

				err := d.Reserve("route-2", t1)

				assert.ErrorIs(t, err, domain.ErrDriverAlreadyReserved)
				assertUnchanged(t, d, from, "route-1", 1)
			})
		}
	}
}

func TestDriverReserveFueraDeLinea(t *testing.T) {
	d := newDriver(t, domain.StatusOffline)

	err := d.Reserve("route-1", t1)

	assert.ErrorIs(t, err, domain.ErrDriverOffline)
	assertUnchanged(t, d, domain.StatusOffline, "", 1)
}

func TestDriverLimpiaLaRutaAlSalirDeLaReserva(t *testing.T) {
	tests := []struct {
		name string
		from domain.Status
		call func(*domain.Driver, time.Time) error
		want domain.Status
	}{
		{"Release", domain.StatusAssigned, releaseRoute1, domain.StatusAvailable},
		{"CompleteRoute", domain.StatusOnRoute, completeRoute1, domain.StatusAvailable},
		{"GoOffline desde ASSIGNED", domain.StatusAssigned, (*domain.Driver).GoOffline, domain.StatusOffline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDriver(t, tt.from)

			require.NoError(t, tt.call(d, t1))

			assert.Equal(t, tt.want, d.Status)
			assert.Empty(t, d.RouteID)
			assert.Equal(t, 2, d.Version)
			assert.Equal(t, t1, d.UpdatedAt)
		})
	}
}

func TestDriverStartRouteConservaLaRuta(t *testing.T) {
	d := newDriver(t, domain.StatusAssigned)

	require.NoError(t, d.StartRoute("route-1", t1))

	assert.Equal(t, "route-1", d.RouteID)
}

func TestDriverTransicionFallidaConservaLaRuta(t *testing.T) {
	d := newDriver(t, domain.StatusOnRoute)

	assert.ErrorIs(t, d.GoOffline(t1), domain.ErrInvalidTransition)
	assert.ErrorIs(t, d.Release("route-1", t1), domain.ErrInvalidTransition)

	assertUnchanged(t, d, domain.StatusOnRoute, "route-1", 1)
}

func TestDriverGoOnlineInactivo(t *testing.T) {
	for _, from := range []domain.Status{domain.StatusAvailable, domain.StatusAssigned, domain.StatusOnRoute, domain.StatusOffline} {
		t.Run(string(from), func(t *testing.T) {
			d := newDriver(t, from)
			d.Active = false
			routeID := d.RouteID

			err := d.GoOnline(t1)

			assert.ErrorIs(t, err, domain.ErrDriverInactive)
			assertUnchanged(t, d, from, routeID, 1)
		})
	}
}

func TestDriverStartRouteInactivo(t *testing.T) {
	for _, from := range []domain.Status{domain.StatusAvailable, domain.StatusAssigned, domain.StatusOnRoute, domain.StatusOffline} {
		t.Run(string(from), func(t *testing.T) {
			d := newDriver(t, from)
			d.Active = false
			routeID := d.RouteID

			err := d.StartRoute("route-1", t1)

			assert.ErrorIs(t, err, domain.ErrDriverInactive)
			assertUnchanged(t, d, from, routeID, 1)
		})
	}
}

func TestDriverTerminarReservaInactivoQuedaFueraDeLinea(t *testing.T) {
	tests := []struct {
		name string
		from domain.Status
		call func(*domain.Driver, time.Time) error
	}{
		{"Release", domain.StatusAssigned, releaseRoute1},
		{"CompleteRoute", domain.StatusOnRoute, completeRoute1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDriver(t, tt.from)
			d.Active = false

			require.NoError(t, tt.call(d, t1))

			assert.Equal(t, domain.StatusOffline, d.Status)
			assert.Empty(t, d.RouteID)
			assert.Equal(t, 2, d.Version)
			assert.Equal(t, t1, d.UpdatedAt)
		})
	}
}

func TestDriverTerminarReservaInactivoDesdeOtroEstado(t *testing.T) {
	tests := []struct {
		name string
		call func(*domain.Driver, time.Time) error
		from []domain.Status
	}{
		{"Release", releaseRoute1, []domain.Status{domain.StatusAvailable, domain.StatusOnRoute, domain.StatusOffline}},
		{"CompleteRoute", completeRoute1, []domain.Status{domain.StatusAvailable, domain.StatusAssigned, domain.StatusOffline}},
	}
	for _, tt := range tests {
		for _, from := range tt.from {
			t.Run(tt.name+" desde "+string(from), func(t *testing.T) {
				d := newDriver(t, from)
				d.Active = false
				routeID := d.RouteID

				err := tt.call(d, t1)

				assert.ErrorIs(t, err, domain.ErrInvalidTransition)
				assertUnchanged(t, d, from, routeID, 1)
			})
		}
	}
}

// routeMethods son las operaciones acotadas a la ruta reservada.
var routeMethods = []struct {
	name string
	call func(*domain.Driver, string, time.Time) error
	from domain.Status
	to   domain.Status
}{
	{"Release", (*domain.Driver).Release, domain.StatusAssigned, domain.StatusAvailable},
	{"StartRoute", (*domain.Driver).StartRoute, domain.StatusAssigned, domain.StatusOnRoute},
	{"CompleteRoute", (*domain.Driver).CompleteRoute, domain.StatusOnRoute, domain.StatusAvailable},
}

func releaseRoute1(d *domain.Driver, now time.Time) error { return d.Release("route-1", now) }

func startRoute1(d *domain.Driver, now time.Time) error { return d.StartRoute("route-1", now) }

func completeRoute1(d *domain.Driver, now time.Time) error { return d.CompleteRoute("route-1", now) }

func TestDriverOperacionesDeRutaExigenRuta(t *testing.T) {
	for _, m := range routeMethods {
		for _, from := range allStatuses {
			for _, active := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s desde %s activo=%t", m.name, from, active), func(t *testing.T) {
					d := newDriver(t, from)
					d.Active = active
					routeID := d.RouteID

					err := m.call(d, "  	", t1)

					assert.ErrorIs(t, err, domain.ErrInvalidRouteID)
					assertUnchanged(t, d, from, routeID, 1)
				})
			}
		}
	}
}

func TestDriverOperacionesDeRutaDesdeOtroEstado(t *testing.T) {
	for _, m := range routeMethods {
		for _, from := range allStatuses {
			if from == m.from {
				continue
			}
			for _, routeID := range []string{"route-1", "route-2"} {
				t.Run(m.name+" desde "+string(from)+" con "+routeID, func(t *testing.T) {
					d := newDriver(t, from)
					current := d.RouteID

					err := m.call(d, routeID, t1)

					assert.ErrorIs(t, err, domain.ErrInvalidTransition)
					assertUnchanged(t, d, from, current, 1)
				})
			}
		}
	}
}

func TestDriverOperacionesDeRutaConOtraRutaSeRechazan(t *testing.T) {
	for _, m := range routeMethods {
		for _, active := range []bool{true, false} {
			if m.name == "StartRoute" && !active {
				continue
			}
			t.Run(fmt.Sprintf("%s activo=%t", m.name, active), func(t *testing.T) {
				d := newDriver(t, m.from)
				d.Active = active

				err := m.call(d, "route-2", t1)

				assert.ErrorIs(t, err, domain.ErrRouteMismatch)
				assertUnchanged(t, d, m.from, "route-1", 1)
			})
		}
	}
}

func TestDriverStartRouteInactivoConOtraRutaDevuelveInactivo(t *testing.T) {
	d := newDriver(t, domain.StatusAssigned)
	d.Active = false

	err := d.StartRoute("route-2", t1)

	assert.ErrorIs(t, err, domain.ErrDriverInactive)
	assertUnchanged(t, d, domain.StatusAssigned, "route-1", 1)
}

func TestDriverOperacionesDeRutaConLaRutaReservada(t *testing.T) {
	for _, m := range routeMethods {
		for _, active := range []bool{true, false} {
			if m.name == "StartRoute" && !active {
				continue
			}
			t.Run(fmt.Sprintf("%s activo=%t", m.name, active), func(t *testing.T) {
				d := newDriver(t, m.from)
				d.Active = active
				want := m.to
				if !active {
					want = domain.StatusOffline
				}

				require.NoError(t, m.call(d, " route-1 ", t1))

				assert.Equal(t, want, d.Status)
				assert.Equal(t, 2, d.Version)
				assert.Equal(t, t1, d.UpdatedAt)
				if want == domain.StatusOnRoute {
					assert.Equal(t, "route-1", d.RouteID)
				} else {
					assert.Empty(t, d.RouteID)
				}
			})
		}
	}
}

func TestDriverReserveEstadoDesconocido(t *testing.T) {
	d := newDriver(t, domain.Status("UNKNOWN"))

	err := d.Reserve("route-1", t1)

	assert.ErrorIs(t, err, domain.ErrInvalidTransition)
	assertUnchanged(t, d, domain.Status("UNKNOWN"), "", 1)
}

func TestDriverReservePrecedenciaConFueraDeLineaInactivo(t *testing.T) {
	d := newDriver(t, domain.StatusOffline)
	d.Active = false

	err := d.Reserve("route-1", t1)

	assert.ErrorIs(t, err, domain.ErrDriverInactive)
	assertUnchanged(t, d, domain.StatusOffline, "", 1)
}

func assertUnchanged(t *testing.T, d *domain.Driver, status domain.Status, routeID string, version int) {
	t.Helper()
	assert.Equal(t, status, d.Status)
	assert.Equal(t, routeID, d.RouteID)
	assert.Equal(t, version, d.Version)
	assert.Equal(t, t0, d.UpdatedAt)
}

func contains(list []domain.Status, s domain.Status) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
