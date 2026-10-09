//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// signalDelay bounds the wait for the signal to terminate the process.
const signalDelay = time.Second

// exitBySignal terminates the process by sig under its default action, so the
// parent observes death by signal. As PID 1 it exits with the shell status of
// sig instead. If the signal does not terminate the process within
// signalDelay, it exits with exitInterrupted.
func exitBySignal(sig os.Signal) {
	if s, ok := sig.(syscall.Signal); ok {
		// The kernel discards a default-action signal sent to PID 1.
		if os.Getpid() == 1 {
			os.Exit(signalExitCode(s))
		}

		signal.Reset(sig)
		_ = syscall.Kill(syscall.Getpid(), s)

		time.Sleep(signalDelay)
	}

	os.Exit(exitInterrupted)
}

// signalExitCode is the shell status of a process killed by s.
func signalExitCode(s syscall.Signal) int {
	return 128 + int(s)
}
