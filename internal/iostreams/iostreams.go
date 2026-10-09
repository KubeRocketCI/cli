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
	return FromWriters(os.Stdin, os.Stdout, os.Stderr)
}

// FromWriters returns an IOStreams over the given streams, with TTY state
// detected from out. Only a terminal *os.File is a TTY.
func FromWriters(in io.Reader, out, errOut io.Writer) *IOStreams {
	return New(in, out, errOut, isTerminal(out))
}

// New returns an IOStreams over the given streams. isTTY is the value
// IsStdoutTTY reports.
func New(in io.Reader, out, errOut io.Writer, isTTY bool) *IOStreams {
	return &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
		isTTY:  isTTY,
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
