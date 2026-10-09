package main

import "os"

// exitBySignal exits with exitInterrupted; Windows has no death by signal.
func exitBySignal(os.Signal) {
	os.Exit(exitInterrupted)
}
