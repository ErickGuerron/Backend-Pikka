// Package migrations embebe las migraciones SQL del esquema orders.
package migrations

import "embed"

// FS contiene los archivos *.sql de este directorio.
//
//go:embed *.sql
var FS embed.FS
