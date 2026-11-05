// Command api is the Linochi ERP backend.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Set at build time with -ldflags (see Makefile). INS-07 shows them in the menu.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args, os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
