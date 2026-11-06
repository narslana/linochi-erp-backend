package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	// Registers the "pgx" database/sql driver that goose uses.
	_ "github.com/jackc/pgx/v5/stdlib"

	dbschema "gitea.lab.internal/linochi/linochi-erp-backend/db"
)

// MigrationStatus is one migration and whether it has been applied.
type MigrationStatus struct {
	Version   int64
	Name      string
	Applied   bool
	AppliedAt time.Time
}

// Migrator runs the embedded goose migrations. The PreSync Job runs `api
// migrate up` with it; the serving process never migrates.
type Migrator struct {
	db       *sql.DB
	provider *goose.Provider
}

// NewMigrator connects to the database at dsn. Close it when done.
func NewMigrator(dsn string) (*Migrator, error) {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	fsys, err := fs.Sub(dbschema.Migrations, "migrations")
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("embedded migrations: %w", err)
	}
	// An advisory lock, so that two runs at once cannot collide.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("session locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("goose provider: %w", err)
	}
	return &Migrator{db: sqlDB, provider: p}, nil
}

// Close releases the connection.
func (m *Migrator) Close() error {
	if err := m.provider.Close(); err != nil {
		return fmt.Errorf("close provider: %w", err)
	}
	return nil
}

// Up applies every pending migration and returns their names, for example
// "20261106120000_users".
func (m *Migrator) Up(ctx context.Context) ([]string, error) {
	results, err := m.provider.Up(ctx)
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, strings.TrimSuffix(r.Source.Path, ".sql"))
	}
	if err != nil {
		return names, fmt.Errorf("migrate up: %w", err)
	}
	return names, nil
}

// Down rolls back the newest applied migration and returns its version. It
// returns ErrNothingToRollBack when nothing is applied. Meant for local use.
func (m *Migrator) Down(ctx context.Context) (int64, error) {
	r, err := m.provider.Down(ctx)
	if err != nil {
		if errors.Is(err, goose.ErrNoNextVersion) || errors.Is(err, goose.ErrNoCurrentVersion) {
			return 0, ErrNothingToRollBack
		}
		return 0, fmt.Errorf("migrate down: %w", err)
	}
	return r.Source.Version, nil
}

// Version returns the newest applied migration, 0 if none.
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	v, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("schema version: %w", err)
	}
	return v, nil
}

// Status lists every embedded migration with its state.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	st, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration status: %w", err)
	}
	out := make([]MigrationStatus, 0, len(st))
	for _, s := range st {
		out = append(out, MigrationStatus{
			Version:   s.Source.Version,
			Name:      strings.TrimSuffix(s.Source.Path, ".sql"),
			Applied:   s.State == goose.StateApplied,
			AppliedAt: s.AppliedAt,
		})
	}
	return out, nil
}

// LatestVersion is the newest migration embedded in this binary. A schema at
// least this new is what the binary needs to serve (see ReadinessCheck).
func LatestVersion() (int64, error) {
	entries, err := fs.ReadDir(dbschema.Migrations, "migrations")
	if err != nil {
		return 0, fmt.Errorf("embedded migrations: %w", err)
	}
	var latest int64
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			continue
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("migration %q has no numeric version prefix: %w", e.Name(), err)
		}
		latest = max(latest, v)
	}
	return latest, nil
}
