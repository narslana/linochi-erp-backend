package oidctest

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
)

// TB is the part of testing.TB the helpers use, so that this package does not
// import testing and cmd/devoidc does not drag it in.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// Server is a Provider listening on a loopback port.
type Server struct {
	*Provider
	// CAPEM is the certificate to trust when the server is TLS (the server's own,
	// which is self-signed). Empty for plain HTTP.
	CAPEM  []byte
	server *httptest.Server
}

// Start serves a Provider over plain HTTP on 127.0.0.1. The issuer is its URL.
func Start(t TB, cfg Config) *Server { t.Helper(); return start(t, cfg, false) }

// StartTLS serves a Provider over HTTPS on 127.0.0.1 with a certificate of its
// own, which is what it takes to test that the backend trusts only the CA it was
// given.
func StartTLS(t TB, cfg Config) *Server { t.Helper(); return start(t, cfg, true) }

func start(t TB, cfg Config, tls bool) *Server {
	t.Helper()
	var prov *Provider
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prov.ServeHTTP(w, r)
	}))
	if tls {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)
	prov = New(srv.URL, cfg)
	s := &Server{Provider: prov, server: srv}
	if tls {
		s.CAPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	}
	return s
}

// CertPool trusts the server's certificate and nothing else.
func (s *Server) CertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(s.server.Certificate())
	return pool
}

// Client is an HTTP client that trusts the server.
func (s *Server) Client() *http.Client { return s.server.Client() }

// Authorize follows the authorization request the backend sent the browser to,
// as the browser would: the provider approves it as its current user and answers
// with a redirect to the callback. It returns that redirect's target.
func (s *Server) Authorize(t TB, authorizeURL string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authorizeURL, nil)
	if err != nil {
		t.Fatalf("authorize request: %v", err)
	}
	c := *s.server.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize answered %d, want a 302 to the callback", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}
