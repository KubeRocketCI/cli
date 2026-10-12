// Package iostreams provides I/O stream abstractions for CLI commands.
package iostreams

import (
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
)

// IOStreams holds the standard I/O streams and TTY state for a command invocation.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
	isTTY  bool
}

// FromWriters returns an IOStreams over the given streams, with TTY state
// detected from out. Only a terminal *os.File is a TTY.
//
// Out and ErrOut are process streams: every write passes the terminalWriter
// policy and then a colour profile that colorprofile.Detect reads once per
// stream from the stream and os.Environ(). Piped output carries no escape
// sequence unless the environment forces a profile (CLICOLOR_FORCE,
// TTY_FORCE). Machine-readable output must escape C1 and DEL itself; the
// stream drops them. IsStdoutTTY is independent of the colour profile.
func FromWriters(in io.Reader, out, errOut io.Writer) *IOStreams {
	return fromWriters(in, out, errOut, os.Environ())
}

func fromWriters(in io.Reader, out, errOut io.Writer, environ []string) *IOStreams {
	return New(in, processStream(out, environ), processStream(errOut, environ), isTerminal(out))
}

// New returns an IOStreams over the given streams as they are, with no
// policy applied. isTTY is the value IsStdoutTTY reports.
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

func processStream(w io.Writer, environ []string) io.Writer {
	return newTerminalWriter(colorprofile.NewWriter(w, environ))
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
