package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// Build-time variables injected via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// SIGINT is registered only when the process did not start with it ignored,
	// so a shell that ignores SIGINT for background jobs keeps that effect.
	sigs := []os.Signal{syscall.SIGTERM}
	if !signal.Ignored(os.Interrupt) {
		sigs = append(sigs, os.Interrupt)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)

	var received atomic.Pointer[os.Signal]

	go func() {
		sig := <-ch
		received.Store(&sig)
		// Stop restores the default action before the command sees the
		// cancellation, so a second signal terminates the process at once.
		signal.Stop(ch)
		cancel()
	}()

	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)

	// Die by the received signal: shells and xargs stop their loops only on
	// death by signal, and SIGTERM reports 143.
	if sig := received.Load(); sig != nil && code == exitInterrupted {
		exitBySignal(*sig)
	}

	os.Exit(code)
}
