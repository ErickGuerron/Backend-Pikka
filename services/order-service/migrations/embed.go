// Package migrations embeds the SQL migrations owned by Order Service.
package migrations

import "embed"

// FS contains the versioned up and down migrations for the orders schema.
//
//go:embed *.sql
var FS embed.FS
