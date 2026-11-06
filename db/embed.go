// Package dbschema embeds the goose migrations, so that the binary carries its
// own schema and `api migrate` needs no files on the pod (NFK-02).
package dbschema

import "embed"

// Migrations holds db/migrations/*.sql.
//
//go:embed migrations/*.sql
var Migrations embed.FS
