// Package migrations embeds the goose SQL migration files.
package migrations

import "embed"

// FS contains all *.sql migrations.
//
//go:embed *.sql
var FS embed.FS
