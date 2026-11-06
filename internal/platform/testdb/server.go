package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Server is one PostgreSQL container shared by all tests in a package. Start it
// in TestMain with StartServer and give every test a database of its own with
// NewDatabase: creating a database takes milliseconds, starting a container
// takes seconds, and on the arm64 runner more.
type Server struct {
	ctr      *postgres.PostgresContainer
	adminDSN string
}

// StartServer starts the container. The caller must call Terminate.
func StartServer(ctx context.Context) (*Server, error) {
	ctr, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("linochi"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		// The container may exist even if the wait failed.
		if ctr != nil {
			_ = testcontainers.TerminateContainer(ctr)
		}
		return nil, fmt.Errorf("start postgres container: %w", err)
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return nil, fmt.Errorf("connection string: %w", err)
	}
	return &Server{ctr: ctr, adminDSN: dsn}, nil
}

// Terminate removes the container.
func (s *Server) Terminate(ctx context.Context) error {
	if err := s.ctr.Terminate(ctx); err != nil {
		return fmt.Errorf("terminate postgres container: %w", err)
	}
	return nil
}

var unsafeName = regexp.MustCompile(`[^a-z0-9_]+`)

// NewDatabase creates an empty database for the test and returns its connection
// string. The database is dropped when the test ends.
func (s *Server) NewDatabase(t testing.TB) string {
	t.Helper()

	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("random: %v", err)
	}
	name := strings.ToLower(unsafeName.ReplaceAllString(strings.ToLower(t.Name()), "_"))
	if len(name) > 40 {
		name = name[:40]
	}
	name = "t_" + name + "_" + hex.EncodeToString(suffix[:])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, s.adminDSN)
	if err != nil {
		t.Fatalf("connect as admin: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatalf("create database: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, s.adminDSN)
		if err != nil {
			t.Errorf("connect to drop %s: %v", name, err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		// FORCE ends connections the test left open.
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
	})

	u, err := url.Parse(s.adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}
