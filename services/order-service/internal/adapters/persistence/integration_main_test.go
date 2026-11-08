//go:build integration

package persistence_test

import (
	"fmt"
	"os"
	"testing"
)

// TestMain aplica el guard a todo el paquete antes de migrar o vaciar tablas: si la variable apunta
// a la base de desarrollo, el paquete entero falla sin tocar datos.
func TestMain(m *testing.M) {
	if err := guardDatabase("ORDER_TEST_DATABASE_URL", os.Getenv("ORDER_TEST_DATABASE_URL")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
