package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// devoidc approves every login without asking. That is only safe where nobody
// else can reach it, so it refuses to listen anywhere but on this machine.
func TestNFK10_DevOIDCRefusesToListenOutsideLoopback(t *testing.T) {
	for _, addr := range []string{
		":9096", "0.0.0.0:9096", "[::]:9096", "192.168.100.5:9096", "10.0.0.1:9096", "auth.lab.internal:9096",
		"localhost.evil.example:9096", "9096", "",
	} {
		var out, errOut bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		code := run(ctx, []string{"-addr", addr}, &out, &errOut)
		cancel()
		if code != 2 {
			t.Errorf("-addr %q: exit %d, want 2 (refused); stdout %q", addr, code, out.String())
		}
		if !strings.Contains(errOut.String(), "loopback") {
			t.Errorf("-addr %q: the refusal does not say why: %q", addr, errOut.String())
		}
	}
}

func TestNFK10_DevOIDCServesAProviderOnLoopbackAndSaysWhatToSetInTheApp(t *testing.T) {
	var out, errOut syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-addr", "127.0.0.1:0", "-redirect", "http://localhost:8080/auth/callback", "-groups", "linochi_erp_users"}, &out, &errOut)
	}()

	var issuer string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && issuer == "" {
		for _, l := range strings.Split(out.String(), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "OIDC_ISSUER="); ok {
				issuer = v
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.HasPrefix(issuer, "http://127.0.0.1:") {
		t.Fatalf("no OIDC_ISSUER on loopback in the output:\n%s%s", out.String(), errOut.String())
	}
	for _, want := range []string{"OIDC_CLIENT_ID=", "OIDC_CLIENT_SECRET=", "OIDC_REDIRECT_URL=http://localhost:8080/auth/callback"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the output does not tell the app to set %s:\n%s", want, out.String())
		}
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err := json.Unmarshal(body, &doc); err != nil || doc["issuer"] != issuer {
		t.Errorf("discovery = %s, err = %v", body, err)
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit %d after cancel, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("devoidc did not stop")
	}
}

// syncBuffer is an output a test reads while the command is still writing.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }
