// Command devoidc is an OpenID Connect provider for local development: it
// approves every login as one fixed user, so that the backend's login can be
// tried without Authelia. It is not part of the image and must never be: it
// signs with a fixed key and asks nobody for anything.
//
// It refuses to listen anywhere but on this machine. Run it next to `make dev`:
//
//	go run ./cmd/devoidc
//
// and set the OIDC_* variables it prints in .env.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without the process globals, so that it can be tested. It returns
// the exit code: 2 for what was asked for, 1 for what went wrong.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("devoidc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:9096", "where to listen: loopback only")
	clientID := fs.String("client-id", "linochi-erp", "the OIDC client id")
	secret := fs.String("client-secret", "dev-only-client-secret", "the OIDC client secret (nothing real)")
	redirect := fs.String("redirect", "http://localhost:8080/auth/callback", "the callback of the app, matched exactly")
	sub := fs.String("sub", oidctest.DefaultUser.Subject, "the subject of the user who logs in")
	email := fs.String("email", oidctest.DefaultUser.Email, "the e-mail address of the user who logs in")
	name := fs.String("name", oidctest.DefaultUser.Name, "the name of the user who logs in")
	groups := fs.String("groups", strings.Join(oidctest.DefaultUser.Groups, ","), "the groups of the user, comma-separated; use none to log in with no role")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	host, _, err := net.SplitHostPort(*addr)
	if err != nil || !isLoopback(host) {
		_, _ = fmt.Fprintf(stderr, "devoidc: -addr %q is not on loopback (127.0.0.1, [::1] or localhost): it approves every login and must not be reachable from anywhere else\n", *addr)
		return 2
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "devoidc: %v\n", err)
		return 1
	}
	issuer := "http://" + ln.Addr().String()
	user := oidctest.User{Subject: *sub, Email: *email, Name: *name, PreferredUsername: strings.SplitN(*email, "@", 2)[0]}
	for g := range strings.SplitSeq(*groups, ",") {
		if g = strings.TrimSpace(g); g != "" && g != "none" {
			user.Groups = append(user.Groups, g)
		}
	}
	p := oidctest.New(issuer, oidctest.Config{ClientID: *clientID, ClientSecret: *secret, RedirectURL: *redirect})
	p.SetUser(user)

	_, _ = fmt.Fprintf(stdout, "devoidc is listening on %s and logs everybody in as %s.\nSet this in the app's environment:\n  OIDC_ISSUER=%s\n  OIDC_CLIENT_ID=%s\n  OIDC_CLIENT_SECRET=%s\n  OIDC_REDIRECT_URL=%s\n",
		issuer, user.Name, issuer, *clientID, *secret, *redirect)

	srv := &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		_, _ = fmt.Fprintf(stderr, "devoidc: %v\n", err)
		return 1
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return 1
	}
	return 0
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
