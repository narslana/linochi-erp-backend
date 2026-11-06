package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/config"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

const migrateUsage = "usage: api migrate up|down|status"

// runMigrate is the `api migrate` subcommand. The PreSync Job runs `api migrate
// up` with the same image as the app; the serving process never migrates. It
// connects with MIGRATE_DATABASE_URL, the database owner, because the role the
// app runs as cannot change the schema. `down` rolls back one migration and is
// meant for local development.
func runMigrate(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) < 3 {
		_, _ = fmt.Fprintln(stderr, migrateUsage)
		return 2
	}
	action := args[2]
	if action != "up" && action != "down" && action != "status" {
		_, _ = fmt.Fprintf(stderr, "unknown migrate action %q\n%s\n", action, migrateUsage)
		return 2
	}

	cfg, err := config.LoadMigrate(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	m, err := db.NewMigrator(cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() { _ = m.Close() }()

	if err := migrate(ctx, m, action, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func migrate(ctx context.Context, m *db.Migrator, action string, stdout io.Writer) error {
	switch action {
	case "up":
		applied, err := m.Up(ctx)
		for _, name := range applied {
			_, _ = fmt.Fprintf(stdout, "applied %s\n", name)
		}
		if err != nil {
			return err //nolint:wrapcheck // already wrapped by db
		}
		if len(applied) == 0 {
			v, err := m.Version(ctx)
			if err != nil {
				return err //nolint:wrapcheck // already wrapped by db
			}
			_, _ = fmt.Fprintf(stdout, "nothing to apply, the schema is at version %d\n", v)
		}
	case "down":
		v, err := m.Down(ctx)
		if errors.Is(err, db.ErrNothingToRollBack) {
			_, _ = fmt.Fprintln(stdout, "nothing to roll back")
			return nil
		}
		if err != nil {
			return err //nolint:wrapcheck // already wrapped by db
		}
		_, _ = fmt.Fprintf(stdout, "rolled back %d\n", v)
	case "status":
		st, err := m.Status(ctx)
		if err != nil {
			return err //nolint:wrapcheck // already wrapped by db
		}
		for _, s := range st {
			state := "pending"
			if s.Applied {
				state = "applied " + s.AppliedAt.Format("2006-01-02 15:04:05")
			}
			_, _ = fmt.Fprintf(stdout, "%-48s %s\n", s.Name, state)
		}
	}
	return nil
}
