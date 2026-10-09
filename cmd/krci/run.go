package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/pkg/cmd/root"
)

// run executes the CLI over stdout and stderr and returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, cmdutil.New(iostreams.FromWriters(os.Stdin, stdout, stderr)))
}

// runWith executes the CLI over f and returns the exit code.
func runWith(ctx context.Context, args []string, f *cmdutil.Factory) int {
	ios := f.IOStreams

	// SetArgs(nil) makes Cobra fall back to os.Args[1:]; an empty slice does not.
	if args == nil {
		args = []string{}
	}

	cmd := root.NewCmdRoot(f, version, commit, date)
	cmd.SetArgs(args)
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)

	configure()

	if err := cmd.ExecuteContext(ctx); err != nil {
		printError(ios.ErrOut, err)

		return 1
	}

	return 0
}

// configure loads defaults, KRCI_* env bindings, and the config file into the
// global viper. Must run before ExecuteContext.
func configure() { config.Init() }

func printError(w io.Writer, err error) { _, _ = fmt.Fprintf(w, "Error: %v\n", err) }
