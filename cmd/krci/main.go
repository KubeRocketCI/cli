// Package main is the entry point for the krci CLI.
package main

import (
	"fmt"
	"os"

	"github.com/KubeRocketCI/cli/internal/cli"
	"github.com/KubeRocketCI/cli/internal/config"
)

// Build-time variables injected via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	config.Init()
	cli.SetVersionInfo(version, commit, date)

	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
