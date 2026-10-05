package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestHandleAuthError(t *testing.T) {
	t.Parallel()

	unauthorized := fmt.Errorf("list: %w", portal.ErrUnauthorized)

	got := HandleAuthError(unauthorized)
	require.ErrorIs(t, got, portal.ErrUnauthorized)
	assert.EqualError(t, got, ErrAuthRequired(unauthorized).Error())

	for _, err := range []error{portal.ErrNotFound, portal.ErrUpstreamUnavailable, errors.New("boom")} {
		assert.Equal(t, err, HandleAuthError(err))
	}
}

func TestPrintError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	tests := []struct {
		name         string
		outputFormat string
		wantStdout   string
	}{
		{
			name:         "json prints the envelope",
			outputFormat: "json",
			wantStdout:   `{"schemaVersion":"7","error":{"message":"boom"}}`,
		},
		{name: "table leaves stdout empty", outputFormat: "table"},
		{name: "default format leaves stdout empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			ios := &iostreams.IOStreams{Out: stdout, ErrOut: &bytes.Buffer{}}

			assert.Equal(t, boom, PrintError(ios, tt.outputFormat, "7", boom))

			if tt.wantStdout == "" {
				assert.Empty(t, stdout.String())

				return
			}

			assert.JSONEq(t, tt.wantStdout, stdout.String())
		})
	}
}

func TestHandleError_EnvelopeCarriesTheLoginHint(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	ios := &iostreams.IOStreams{Out: stdout, ErrOut: &bytes.Buffer{}}

	got := HandleError(ios, "json", "7", portal.ErrUnauthorized)
	require.ErrorIs(t, got, portal.ErrUnauthorized)

	want, err := json.Marshal(map[string]any{
		"schemaVersion": "7",
		"error":         map[string]string{"message": ErrAuthRequired(portal.ErrUnauthorized).Error()},
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(want), stdout.String())
}
