// Package db opens the PostgreSQL pool, runs the embedded migrations and holds
// the typed errors and helpers shared by every module's store.
package db

import "errors"

var (
	// ErrNotFound means the row does not exist. httpx maps it to 404.
	ErrNotFound = errors.New("not found")
	// ErrStaleVersion means the row exists but the caller wrote with an old
	// version: someone else changed it first (optimistic locking). httpx maps it
	// to 409 so that the client can show the latest version instead of
	// overwriting it.
	ErrStaleVersion = errors.New("stale version")
	// ErrNothingToRollBack is returned by Migrator.Down when no migration is applied.
	ErrNothingToRollBack = errors.New("nothing to roll back")
)
