// Package iostreams provides I/O stream abstractions for CLI commands.
package iostreams

import (
	"io"
	"os"
)

// IOStreams holds the standard I/O streams and TTY state for a command invocation.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
	isTTY  bool
}

// System returns an IOStreams wired to the real os.Stdin/Stdout/Stderr,
// with TTY state detected from os.Stdout.
func System() *IOStreams {
	return &IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
		isTTY:  isTerminal(os.Stdout),
	}
}

// IsStdoutTTY reports whether Out is connected to an interactive terminal.
func (s *IOStreams) IsStdoutTTY() bool {
	return s.isTTY
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	info, err := f.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
