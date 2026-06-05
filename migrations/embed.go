// Package migrations embeds the SQL schema migrations into the binary so the
// service can apply them on startup without shipping the .sql files separately.
package migrations

import "embed"

// Files holds every migration, applied in lexical order by the migration runner.
//
//go:embed *.sql
var Files embed.FS
