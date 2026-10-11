package token

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// mockKeyring makes every keyring call fail with err. go-keyring has no way to
// restore the previous provider; the mock stays installed for the test binary.
func mockKeyring(t *testing.T, err error) {
	t.Helper()

	keyring.MockInitWithError(err)
	t.Setenv("KRCI_KEYRING_BACKEND", "")
}

func TestKeyringFallbackWarning(t *testing.T) {
	tests := []struct {
		name       string
		keyringErr error
	}{
		{"keyring read fails", errors.New("keyring locked")},
		{"key absent and keyring write fails", keyring.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKeyring(t, tt.keyringErr)

			dir := t.TempDir()
			keyPath := filepath.Join(dir, ".keyfile")

			var buf bytes.Buffer
			enc := NewAESEncryptor("krci-test", dir, &buf)

			_, err := enc.Encrypt([]byte("payload"))
			require.NoError(t, err)

			want := fmt.Sprintf("Warning: OS keyring unavailable, using file-based key storage.\n"+
				"  Ensure %s is protected.\n", keyPath)
			assert.Equal(t, want, buf.String())
			assert.FileExists(t, keyPath)
		})
	}
}

func TestKeyFilePermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("key file permissions are not checked on Windows")
	}

	tests := []struct {
		name string
		mode os.FileMode
		want func(keyPath string) string
	}{
		{
			name: "group and world readable",
			mode: 0o644,
			want: func(keyPath string) string {
				return fmt.Sprintf("Warning: %s has permissions 0644, expected 0600.\n"+
					"  Run: chmod 600 %s\n", keyPath, keyPath)
			},
		},
		{
			name: "owner only",
			mode: 0o600,
			want: func(string) string { return "" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KRCI_KEYRING_BACKEND", "file")

			dir := t.TempDir()
			keyPath := filepath.Join(dir, ".keyfile")
			encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32))
			require.NoError(t, os.WriteFile(keyPath, []byte(encoded), tt.mode))
			require.NoError(t, os.Chmod(keyPath, tt.mode))

			var buf bytes.Buffer
			enc := NewAESEncryptor("krci-test", dir, &buf)

			_, err := enc.Encrypt([]byte("payload"))
			require.NoError(t, err)

			assert.Equal(t, tt.want(keyPath), buf.String())
		})
	}
}

func TestNewAESEncryptorNilWriterPanics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "token.NewAESEncryptor: nil writer", func() {
		NewAESEncryptor("krci-test", t.TempDir(), nil)
	})
}
