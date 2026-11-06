//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

var pgServer *testdb.Server

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := testdb.StartServer(ctx)
		if err != nil {
			_, _ = os.Stderr.WriteString("start postgres: " + err.Error() + "\n")
			return 1
		}
		pgServer = s
		defer func() { _ = s.Terminate(context.Background()) }()
		return m.Run()
	}())
}

func TestNFK28_MigrateUpStatusDown(t *testing.T) {
	env := map[string]string{"MIGRATE_DATABASE_URL": pgServer.NewDatabase(t)}

	if code, out, errOut := migrateCmd(t, env, "status"); code != 0 || !strings.Contains(out, "pending") {
		t.Fatalf("status before up: code=%d out=%q err=%q, want the migration listed as pending", code, out, errOut)
	}
	if code, out, errOut := migrateCmd(t, env, "up"); code != 0 || !strings.Contains(out, "applied") || !strings.Contains(out, "users") {
		t.Fatalf("up: code=%d out=%q err=%q", code, out, errOut)
	}
	if code, out, _ := migrateCmd(t, env, "up"); code != 0 || !strings.Contains(out, "nothing to apply") {
		t.Fatalf("second up: code=%d out=%q, want nothing to apply", code, out)
	}
	if code, out, _ := migrateCmd(t, env, "status"); code != 0 || strings.Contains(out, "pending") {
		t.Fatalf("status after up: code=%d out=%q, want everything applied", code, out)
	}
	if code, out, errOut := migrateCmd(t, env, "down"); code != 0 || !strings.Contains(out, "rolled back") {
		t.Fatalf("down: code=%d out=%q err=%q", code, out, errOut)
	}
	// One step at a time until nothing is left.
	for i := 0; ; i++ {
		code, out, _ := migrateCmd(t, env, "down")
		if code != 0 {
			t.Fatalf("down #%d: code=%d out=%q", i+2, code, out)
		}
		if strings.Contains(out, "nothing to roll back") {
			break
		}
		if i > 10 {
			t.Fatal("down never reached the start")
		}
	}
}

// freeAddr returns a loopback address that is free right now.
func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

type readyz struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func fetch(t *testing.T, url string) (int, readyz) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, readyz{}
	}
	defer func() { _ = resp.Body.Close() }()
	var body readyz
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

// start runs the whole app and returns the internal address and a function that
// stops it and returns the exit code.
func start(t *testing.T, databaseURL string) (internal string, stop func() int) {
	t.Helper()
	internal = freeAddr(t)
	env := map[string]string{
		"DATABASE_URL": databaseURL, "HTTP_ADDR": freeAddr(t), "INTERNAL_ADDR": internal,
		"SHUTDOWN_DELAY": "0s", "DB_MAX_CONNS": "2",
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"api"}, envWith(env), os.Stdout, os.Stderr) }()
	t.Cleanup(cancel)

	// Wait until the internal port answers at all.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if code, _ := fetch(t, "http://"+internal+"/healthz"); code == 200 {
			return internal, func() int {
				cancel()
				select {
				case c := <-done:
					return c
				case <-time.After(10 * time.Second):
					return -1
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the app never started serving /healthz")
	return "", nil
}

func TestNFK02_ReadyzIsGreenWhenTheDatabaseIsMigrated(t *testing.T) {
	url := pgServer.NewDatabase(t)
	if code, _, errOut := migrateCmd(t, map[string]string{"MIGRATE_DATABASE_URL": url}, "up"); code != 0 {
		t.Fatalf("migrate up: %d %s", code, errOut)
	}
	internal, stop := start(t, testdb.AppURL(url))

	deadline := time.Now().Add(10 * time.Second)
	var code int
	var body readyz
	for time.Now().Before(deadline) {
		if code, body = fetch(t, "http://"+internal+"/readyz"); code == 200 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if code != 200 || body.Checks["database"] != "ok" {
		t.Fatalf("/readyz = %d %+v, want 200 with database ok", code, body)
	}
	if exit := stop(); exit != 0 {
		t.Errorf("exit code %d after shutdown, want 0", exit)
	}
}

// A schema that is behind the binary means this version must not get traffic,
// but the process still starts and /healthz stays green (no crash loop).
func TestNFK02_ReadyzIsRedUntilTheSchemaIsMigratedAndTheAppStillStarts(t *testing.T) {
	internal, stop := start(t, testdb.AppURL(pgServer.NewDatabase(t))) // empty database, no migrations
	code, body := fetch(t, "http://"+internal+"/readyz")
	if code != http.StatusServiceUnavailable || body.Checks["database"] != "failed" {
		t.Fatalf("/readyz = %d %+v, want 503 with database failed", code, body)
	}
	if code, _ := fetch(t, "http://"+internal+"/healthz"); code != 200 {
		t.Errorf("/healthz = %d, want 200: liveness must not depend on the database", code)
	}
	if exit := stop(); exit != 0 {
		t.Errorf("exit code %d, want 0", exit)
	}
}

func TestNFK02_TheAppStartsWhileTheDatabaseIsDown(t *testing.T) {
	internal, stop := start(t, dsn)
	if code, body := fetch(t, "http://"+internal+"/readyz"); code != http.StatusServiceUnavailable || body.Checks["database"] != "failed" {
		t.Fatalf("/readyz = %d %+v, want 503 with database failed", code, body)
	}
	if exit := stop(); exit != 0 {
		t.Errorf("exit code %d, want 0", exit)
	}
}
