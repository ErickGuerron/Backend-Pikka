package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "normaliza mayúsculas y espacios", in: "  Ana@Example.COM ", want: "ana@example.com"},
		{name: "vacío", in: "", wantErr: true},
		{name: "sin arroba", in: "ana.example.com", wantErr: true},
		{name: "sin dominio con punto", in: "ana@localhost", wantErr: true},
		{name: "con nombre visible", in: "Ana <ana@example.com>", wantErr: true},
		{name: "demasiado largo", in: strings.Repeat("a", 250) + "@x.io", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NormalizeEmail(tt.in)
			if tt.wantErr {
				assert.ErrorIs(t, err, domain.ErrInvalidEmail)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidatePassword(t *testing.T) {
	assert.ErrorIs(t, domain.ValidatePassword("corta"), domain.ErrInvalidPassword)
	assert.ErrorIs(t, domain.ValidatePassword(strings.Repeat("x", 73)), domain.ErrInvalidPassword)
	assert.NoError(t, domain.ValidatePassword("suficiente"))
}

func TestNewUserInputValidate(t *testing.T) {
	valid := domain.NewUserInput{Email: "op@example.com", Password: "password123", FullName: " Operador Uno ", Role: domain.RoleOperator}

	got, err := valid.Validate()
	require.NoError(t, err)
	assert.Equal(t, "Operador Uno", got.FullName)

	noName := valid
	noName.FullName = "   "
	_, err = noName.Validate()
	assert.ErrorIs(t, err, domain.ErrInvalidFullName)

	badRole := valid
	badRole.Role = "CUSTOMER"
	_, err = badRole.Validate()
	assert.ErrorIs(t, err, domain.ErrInvalidRole)
}

func TestParseRole(t *testing.T) {
	r, err := domain.ParseRole("DRIVER")
	require.NoError(t, err)
	assert.Equal(t, domain.RoleDriver, r)

	_, err = domain.ParseRole("driver")
	assert.ErrorIs(t, err, domain.ErrInvalidRole)
}
