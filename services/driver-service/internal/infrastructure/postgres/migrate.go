// Package postgres implementa el repositorio de repartidores y aplica sus migraciones.
package postgres

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5 database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/migrations"
)

// Migrate aplica las migraciones pendientes. El DSN debe seleccionar el esquema drivers.
// Ejecutarla de nuevo sin cambios no falla.
func Migrate(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	databaseURL := "pgx5://" + strings.TrimPrefix(strings.TrimPrefix(dsn, "postgres://"), "postgresql://")
	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return errors.Join(fmt.Errorf("init driver migrations: %w", err), src.Close())
	}
	upErr := m.Up()
	srcErr, dbErr := m.Close()
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return errors.Join(fmt.Errorf("apply driver migrations: %w", upErr), srcErr, dbErr)
	}
	return errors.Join(srcErr, dbErr)
}
