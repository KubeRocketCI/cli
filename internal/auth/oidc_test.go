package auth

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pkg/browser"
	"github.com/stretchr/testify/assert"
)

// TestOpenAuthURL mutates browser.Stdout and browser.Stderr; it must not run in parallel.
func TestOpenAuthURL(t *testing.T) {
	prevStdout, prevStderr := browser.Stdout, browser.Stderr

	t.Cleanup(func() {
		browser.Stdout, browser.Stderr = prevStdout, prevStderr
	})

	const authURL = "https://idp.example.com/auth?state=abc"

	instructions := "Opening browser for authentication...\n" +
		"If browser doesn't open, visit:\n  " + authURL + "\n\n"

	tests := []struct {
		name    string
		openErr error
		want    string
	}{
		{
			name: "browser opens",
			want: instructions,
		},
		{
			name:    "browser fails",
			openErr: errors.New("exec: \"xdg-open\": executable file not found in $PATH"),
			want: instructions +
				"Warning: could not open browser: exec: \"xdg-open\": executable file not found in $PATH\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				buf    bytes.Buffer
				opened string
			)

			openAuthURL(&buf, authURL, func(u string) error {
				opened = u
				assert.Equal(t, instructions, buf.String())

				return tt.openErr
			})

			assert.Equal(t, authURL, opened)
			assert.Equal(t, tt.want, buf.String())
			assert.Same(t, &buf, browser.Stdout)
			assert.Same(t, &buf, browser.Stderr)
		})
	}
}
