package testdb

import (
	"context"
	"testing"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// NewMigratedDatabase is NewDatabase followed by all migrations, run as the
// owner like the PreSync Job does. It returns the owner's connection string;
// use AppURL for the restricted role the app runs as.
func (s *Server) NewMigratedDatabase(t testing.TB) string {
	t.Helper()
	dsn := s.NewDatabase(t)
	m, err := db.NewMigrator(dsn)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	defer func() { _ = m.Close() }()
	if _, err := m.Up(context.Background()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return dsn
}
