package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

func TestUpdateUser(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	uc := application.NewUpdateUser(repo)
	driver := seedUser(t, repo, "driver@example.com", domain.RoleDriver, true)
	seedUser(t, repo, "admin2@example.com", domain.RoleAdmin, true)
	customer := seedUser(t, repo, "cliente@example.com", domain.RoleCustomer, true)
	name := "Driver Renombrado"

	t.Run("solo ADMIN puede editar", func(t *testing.T) {
		_, err := uc.Execute(ctx, ports.Caller{UserID: "x", Role: domain.RoleOperator}, driver.ID, domain.UserUpdate{FullName: &name})
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("edita nombre y persiste con version+1", func(t *testing.T) {
		got, err := uc.Execute(ctx, admin, driver.ID, domain.UserUpdate{FullName: &name})
		require.NoError(t, err)
		assert.Equal(t, name, got.FullName)
		assert.Equal(t, 2, got.Version)

		stored, err := repo.FindByID(ctx, driver.ID)
		require.NoError(t, err)
		assert.Equal(t, name, stored.FullName)
	})

	t.Run("usuario inexistente", func(t *testing.T) {
		_, err := uc.Execute(ctx, admin, "user-999", domain.UserUpdate{FullName: &name})
		assert.ErrorIs(t, err, domain.ErrUserNotFound)
	})

	t.Run("CUSTOMER no cambia de rol", func(t *testing.T) {
		role := domain.RoleDriver
		_, err := uc.Execute(ctx, admin, customer.ID, domain.UserUpdate{Role: &role})
		assert.ErrorIs(t, err, domain.ErrCustomerRoleLocked)
	})

	t.Run("no se puede asignar CUSTOMER", func(t *testing.T) {
		role := domain.RoleCustomer
		_, err := uc.Execute(ctx, admin, driver.ID, domain.UserUpdate{Role: &role})
		assert.ErrorIs(t, err, domain.ErrCustomerRoleNotAssignable)
	})
}

func TestDeactivateUser(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	uc := application.NewDeactivateUser(repo)
	adminA := seedUser(t, repo, "admin-a@example.com", domain.RoleAdmin, true)
	adminB := seedUser(t, repo, "admin-b@example.com", domain.RoleAdmin, true)
	driver := seedUser(t, repo, "driver@example.com", domain.RoleDriver, true)
	actorA := ports.Caller{UserID: adminA.ID, Role: domain.RoleAdmin}

	t.Run("solo ADMIN puede desactivar", func(t *testing.T) {
		_, err := uc.Execute(ctx, ports.Caller{UserID: "x", Role: domain.RoleDriver}, driver.ID)
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("no puede desactivarse a sí mismo", func(t *testing.T) {
		_, err := uc.Execute(ctx, actorA, adminA.ID)
		assert.ErrorIs(t, err, domain.ErrSelfDeactivation)
	})

	t.Run("desactiva con borrado lógico y version+1", func(t *testing.T) {
		got, err := uc.Execute(ctx, actorA, driver.ID)
		require.NoError(t, err)
		assert.False(t, got.Active)
		assert.Equal(t, 2, got.Version)

		stored, err := repo.FindByID(ctx, driver.ID)
		require.NoError(t, err)
		assert.False(t, stored.Active)
	})

	t.Run("no deja el sistema sin ADMIN activo", func(t *testing.T) {
		_, err := uc.Execute(ctx, actorA, adminB.ID)
		require.NoError(t, err, "quedan dos ADMIN activos antes de la baja")

		// Ahora adminA es el único ADMIN activo: un tercero no puede darlo de baja.
		_, err = uc.Execute(ctx, ports.Caller{UserID: "u-externo", Role: domain.RoleAdmin}, adminA.ID)
		assert.ErrorIs(t, err, domain.ErrLastActiveAdmin)
	})

	t.Run("usuario inexistente", func(t *testing.T) {
		_, err := uc.Execute(ctx, actorA, "user-999")
		assert.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}
