package postgres_test

import (
	"net/url"
	"strings"
	"testing"
)

// protectedDatabase es la base de desarrollo compartida. Las pruebas de integración vacían tablas
// (TRUNCATE) y no deben correr jamás contra ella.
const protectedDatabase = "last_mile"

// refuseProtectedDatabase falla la prueba si el DSN apunta a la base de desarrollo.
func refuseProtectedDatabase(t *testing.T, dsn string) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("DRIVER_TEST_DATABASE_URL is not a valid URL: %v", err)
	}
	if strings.EqualFold(strings.TrimPrefix(u.Path, "/"), protectedDatabase) {
		t.Fatalf("DRIVER_TEST_DATABASE_URL points at the development database %q; use a disposable database", protectedDatabase)
	}
}
