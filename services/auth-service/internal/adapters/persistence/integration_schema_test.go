//go:build integration

package persistence_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// isolatedDSN crea un esquema nuevo para la prueba y devuelve el DSN apuntando a el. Cada prueba tiene
// sus propias tablas: no comparte estado con otras y no hace falta vaciar nada (nada se borra fisicamente).
func isolatedDSN(t *testing.T, envName string) string {
	t.Helper()
	base := os.Getenv(envName)
	if base == "" {
		t.Skip(envName + " not set")
	}
	schema := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	require.NoError(t, err)
	defer admin.Close()
	_, err = admin.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema))
	require.NoError(t, err)
	u, err := url.Parse(base)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}
