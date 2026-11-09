// Package migrations embeds the workshop database migrations.
package migrations

import "embed"

// FS contains the workshop database migrations.
//
//go:embed *.sql
var FS embed.FS
