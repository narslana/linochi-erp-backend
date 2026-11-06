//go:build integration

package testdb_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

// Connecting over the network from the test process to the container is the
// part that breaks in Docker-in-Docker (host and port mapping), so this test
// deliberately does not just run pg_isready inside the container.
func TestNFK28_PostgresContainerAcceptsConnections(t *testing.T) {
	dsn := testdb.Start(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var version int
	if err := conn.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil {
		t.Fatalf("query: %v", err)
	}
	// The cluster runs PostgreSQL 18 (see BESLUT.md), and tests must match it.
	if version < 180000 || version >= 190000 {
		t.Errorf("server_version_num = %d, want a PostgreSQL 18.x", version)
	}
}
