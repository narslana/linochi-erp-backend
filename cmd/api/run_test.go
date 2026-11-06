package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// A request that is in flight when SIGTERM arrives must still get its answer,
// otherwise every rolling update drops requests (NFK-02).
func TestNFK02_ServerShutsDownGracefullyOnCancel(t *testing.T) {
	ln := listen(t)

	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serve(ctx, discardLogger(), ln, handler, 5*time.Second) }()

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		got <- result{body: string(b), err: err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started")
	}

	cancel() // what SIGTERM does

	select {
	case err := <-served:
		t.Fatalf("serve returned before the in-flight request finished: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)

	select {
	case r := <-got:
		if r.err != nil || r.body != "done" {
			t.Fatalf("in-flight request: body=%q err=%v", r.body, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no response for the in-flight request")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after shutdown")
	}
}

func TestNFK02_ServerShutdownIsBoundedByTimeout(t *testing.T) {
	ln := listen(t)
	started := make(chan struct{})
	stuck := make(chan struct{})
	defer close(stuck)
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-stuck
	})

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, discardLogger(), ln, handler, 100*time.Millisecond) }()

	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String(), nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	cancel()

	select {
	case err := <-served:
		if err == nil {
			t.Fatal("expected a shutdown timeout error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve ignored the shutdown timeout")
	}
}

func TestINS07_VersionSubcommandPrintsLdflagsValues(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"api", "version"}, func(string) string { return "" }, &out, io.Discard)
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got, want := strings.TrimSpace(out.String()), "dev · unknown"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNFK02_RunFailsWithConfigErrorNamingTheField(t *testing.T) {
	var errOut bytes.Buffer
	code := run(context.Background(), []string{"api"}, func(string) string { return "" }, io.Discard, &errOut)
	if code == 0 {
		t.Fatal("expected a non-zero exit code without DATABASE_URL")
	}
	if !strings.Contains(errOut.String(), "DATABASE_URL") {
		t.Errorf("stderr should name the field: %q", errOut.String())
	}
}
