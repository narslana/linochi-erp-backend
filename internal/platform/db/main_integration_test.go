//go:build integration

package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

// One PostgreSQL container for the whole package, and a database per test.
var server *testdb.Server

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := testdb.StartServer(ctx)
		if err != nil {
			_, _ = os.Stderr.WriteString("start postgres: " + err.Error() + "\n")
			return 1
		}
		server = s
		defer func() { _ = s.Terminate(context.Background()) }()
		return m.Run()
	}())
}
