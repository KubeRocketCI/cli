package cmdtest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/output"
)

// RunCmd builds the command with newCmd, executes it with argv, and returns
// the options runF captured. Validation runs; the network does not.
func RunCmd[T any](
	t *testing.T,
	newCmd func(*cmdutil.Factory, func(*T) error) *cobra.Command,
	argv []string,
) (*T, error) {
	t.Helper()

	var captured *T

	cmd := newCmd(NewFactory(), func(o *T) error {
		captured = o

		return nil
	})

	cmd.SetArgs(argv)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	return captured, cmd.Execute()
}

// Execute runs the command built by newCmd with its own run function, the
// factory f and argv, and returns the error the command exits with.
func Execute[T any](
	newCmd func(*cmdutil.Factory, func(*T) error) *cobra.Command,
	f *cmdutil.Factory,
	argv []string,
) error {
	cmd := newCmd(f, nil)

	cmd.SetArgs(argv)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	return cmd.Execute()
}

// DecodeErrorEnvelope returns the JSON error envelope a verb wrote to stdout;
// the test fails when stdout holds anything else.
func DecodeErrorEnvelope(t *testing.T, stdout *bytes.Buffer) output.JSONErrorEnvelope {
	t.Helper()

	var env output.JSONErrorEnvelope

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&env); err != nil || dec.More() {
		t.Fatalf("stdout is not one JSON error envelope (decode error: %v):\n%s", err, stdout)
	}

	return env
}

// CheckErrorOutput drives the verb's own run function into an error against a
// mock portal served by handler and checks what reaches stdout: the JSON error
// envelope with the message want under -o json, nothing without it.
func CheckErrorOutput[T any](
	t *testing.T,
	newCmd func(*cmdutil.Factory, func(*T) error) *cobra.Command,
	handler http.Handler,
	argv []string,
	schemaVersion, want string,
) {
	t.Helper()

	for _, format := range []string{output.FormatJSON, ""} {
		f, stdout := NewPortalFactory(t, handler)

		args := argv
		if format != "" {
			args = append(slices.Clone(argv), "-o", format)
		}

		err := Execute(newCmd, f, args)
		if err == nil || err.Error() != want {
			t.Fatalf("-o %q: want error %q, got %v", format, want, err)
		}

		if format == "" {
			if stdout.Len() != 0 {
				t.Errorf("without -o json stdout must stay empty, got %q", stdout.String())
			}

			continue
		}

		wantEnvelope := output.JSONErrorEnvelope{
			SchemaVersion: schemaVersion,
			Error:         output.JSONErrorBody{Message: want},
		}
		if got := DecodeErrorEnvelope(t, stdout); got != wantEnvelope {
			t.Errorf("envelope = %+v, want %+v", got, wantEnvelope)
		}
	}
}
