package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadinessCheck is the /readyz check for the database: the pool must answer,
// and the schema must be at least as new as the migrations embedded in this
// binary. A schema that is newer is fine, so that a rollback to the previous
// image keeps working (migrations must be backwards compatible, see the
// release skill).
func ReadinessCheck(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping database: %w", err)
		}
		latest, err := LatestVersion()
		if err != nil {
			return err
		}
		var current int64
		// No goose_db_version table at all also lands here: not migrated, not ready.
		if err := pool.QueryRow(ctx, `-- name: ReadinessSchemaVersion :one
SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&current); err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		if current < latest {
			return fmt.Errorf("schema version %d is behind the binary's %d: run `api migrate up`", current, latest)
		}
		return nil
	}
}
