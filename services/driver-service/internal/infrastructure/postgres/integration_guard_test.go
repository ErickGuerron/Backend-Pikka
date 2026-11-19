package postgres_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// protectedDatabase es la base de desarrollo compartida. Las pruebas de integración vacían tablas
// (TRUNCATE) y no deben correr jamás contra ella.
const protectedDatabase = "last_mile"

// guardDatabase devuelve error si el DSN apunta a la base de desarrollo. Un DSN vacío no se valida:
// sin variable las pruebas de integración se omiten.
func guardDatabase(envName, dsn string) error {
	if dsn == "" {
		return nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", envName, err)
	}
	if strings.EqualFold(strings.TrimPrefix(u.Path, "/"), protectedDatabase) {
		return fmt.Errorf("%s points at the development database %q; use a disposable database", envName, protectedDatabase)
	}
	return nil
}

// TestGuardDatabase fija la regla: las pruebas de integración vacían tablas y no deben correr
// contra la base de desarrollo compartida, sin importar el esquema o los parámetros del DSN.
func TestGuardDatabase(t *testing.T) {
	cases := []struct {
		name    string
		dsn     string
		refused bool
	}{
		{"development database", "postgres://u:p@localhost:5432/last_mile?sslmode=disable", true},
		{"development database upper case", "postgres://u:p@localhost:5432/LAST_MILE", true},
		{"development database with search path", "postgres://u:p@localhost:5432/last_mile?search_path=auth", true},
		{"disposable database", "postgres://u:p@localhost:5432/last_mile_test?sslmode=disable", false},
		{"unparseable url is refused", "postgres://%zz", true},
		{"empty means tests are skipped", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := guardDatabase("DRIVER_TEST_DATABASE_URL", tt.dsn)
			if tt.refused && err == nil {
				t.Fatalf("guardDatabase(%q) = nil, want refusal", tt.dsn)
			}
			if !tt.refused && err != nil {
				t.Fatalf("guardDatabase(%q) = %v, want nil", tt.dsn, err)
			}
		})
	}
}
