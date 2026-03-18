// Package main is the entry point for the krci CLI.
package main

import (
	"fmt"
	"os"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/pkg/cmd/root"
)

// Build-time variables injected via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	config.Init()

	f := cmdutil.New()

	if err := root.NewCmdRoot(f, version, commit, date).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
