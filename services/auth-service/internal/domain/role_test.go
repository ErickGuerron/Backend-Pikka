package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

func TestRole_Customer_IsValid(t *testing.T) {
	assert.True(t, domain.Role("CUSTOMER").Valid(), "ROLE_CUSTOMER debe ser válido")
}

func TestRole_Admin_Operator_Driver_StillValid(t *testing.T) {
	cases := []domain.Role{domain.RoleAdmin, domain.RoleOperator, domain.RoleDriver}
	for _, r := range cases {
		assert.Truef(t, r.Valid(), "rol %q debe seguir siendo válido", string(r))
	}
}

func TestRole_Unknown_IsInvalid(t *testing.T) {
	assert.False(t, domain.Role("INVALID").Valid(), "rol desconocido debe ser inválido")
}
