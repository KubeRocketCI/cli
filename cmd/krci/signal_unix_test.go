//go:build !windows

package main

import (
	"syscall"
	"testing"
)

func TestSignalExitCode(t *testing.T) {
	t.Parallel()

	for sig, want := range map[syscall.Signal]int{syscall.SIGINT: exitInterrupted, syscall.SIGTERM: 143} {
		if got := signalExitCode(sig); got != want {
			t.Errorf("signalExitCode(%v) = %d, want %d", sig, got, want)
		}
	}
}
