// Package testdb starts a throwaway PostgreSQL for integration tests (NFK-28).
//
// It must never assume the container is on localhost: in CI the tests run in a
// container next to a Docker-in-Docker daemon, where published ports are
// reached through the daemon's host. The testcontainers module resolves that.
package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image is the same major and minor as the CNPG cluster in the homelab
// (ghcr.io/cloudnative-pg/postgresql:18.6-system-trixie), on the same Debian
// release, so collation and locale behave as in production. Change it together
// with the cluster, and with compose.yaml.
const Image = "postgres:18.6-trixie"

// Start runs a PostgreSQL container for the duration of the test and returns a
// connection string. The container is removed when the test ends.
func Start(t testing.TB) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ctr, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("linochi_erp_test"),
		postgres.WithUsername("linochi"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}
