// Package migrations embeds the auth database migrations.
package migrations

import "embed"

// FS contains the auth database migrations.
//
//go:embed *.sql
var FS embed.FS
