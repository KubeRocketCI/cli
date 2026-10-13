package token

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// mockKeyring makes every keyring call fail with err; a nil err installs a
// working, empty in-memory keyring. go-keyring has no way to restore the
// previous provider; the mock stays installed for the test binary.
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

// saveTestToken writes a token encrypted with the testEncryptor key to dir and
// returns its path. withKeyFile also stores that key in .keyfile.
func saveTestToken(t *testing.T, dir string, withKeyFile bool) string {
	t.Helper()

	path := filepath.Join(dir, "tokens.enc")
	require.NoError(t, NewEncryptedStore(path, newTestEncryptor(), io.Discard).Save(&StoredToken{AccessToken: "kept"}))

	if withKeyFile {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".keyfile"), []byte(encodeKey(newTestEncryptor().key)), 0o600))
	}

	return path
}

func TestLoadEmptyKeyringReadsKeyFile(t *testing.T) {
	mockKeyring(t, nil)

	dir := t.TempDir()
	path := saveTestToken(t, dir, true)

	var buf bytes.Buffer
	store := NewEncryptedStore(path, NewAESEncryptor("krci-test", dir, &buf), &buf)

	tok, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "kept", tok.AccessToken)
	assert.Empty(t, buf.String())

	_, err = keyring.Get("krci-test", keyringUser)
	require.ErrorIs(t, err, keyring.ErrNotFound, "Load stored a new key in the keyring")
}

func TestLoadWithoutStoredKeyKeepsFile(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{name: "empty keyring", setup: func(t *testing.T) { mockKeyring(t, nil) }},
		{name: "file backend", setup: func(t *testing.T) { t.Setenv("KRCI_KEYRING_BACKEND", "file") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			dir := t.TempDir()
			path := saveTestToken(t, dir, false)

			var buf bytes.Buffer
			enc := NewAESEncryptor("krci-test", dir, &buf)
			store := NewEncryptedStore(path, enc, &buf)

			_, err := enc.Decrypt(bytes.Repeat([]byte("x"), minCiphertextLen))
			require.ErrorIs(t, err, ErrKeyNotFound)

			for range 3 {
				_, err = store.Load()
				require.ErrorIs(t, err, ErrNoToken)
			}

			assert.FileExists(t, path)
			assert.NoFileExists(t, filepath.Join(dir, ".keyfile"), "Load created a key file")
			assert.Equal(t, fmt.Sprintf("Warning: no encryption key for the token file (%s); "+
				"the next login replaces it.\n", path), buf.String())

			require.NoError(t, store.Save(&StoredToken{AccessToken: "fresh"}))

			tok, err := store.Load()
			require.NoError(t, err)
			assert.Equal(t, "fresh", tok.AccessToken)
		})
	}
}

func TestLoadKeyringUnavailableKeepsFile(t *testing.T) {
	mockKeyring(t, errors.New("keyring locked"))

	dir := t.TempDir()
	path := saveTestToken(t, dir, false)

	var buf bytes.Buffer
	store := NewEncryptedStore(path, NewAESEncryptor("krci-test", dir, &buf), &buf)

	_, err := store.Load()
	require.ErrorIs(t, err, ErrKeyringAccess)
	require.EqualError(t, err, "decrypting token: getting encryption key: unable to access OS keyring: keyring locked")
	assert.FileExists(t, path)
	assert.NoFileExists(t, filepath.Join(dir, ".keyfile"), "Load created a key file")
	assert.Empty(t, buf.String(), "no file-based key storage is in use")
}

func TestLoadKeyringUnavailableMalformedKeyFileKeepsFile(t *testing.T) {
	mockKeyring(t, errors.New("keyring locked"))

	dir := t.TempDir()
	path := saveTestToken(t, dir, false)
	keyPath := filepath.Join(dir, ".keyfile")
	require.NoError(t, os.WriteFile(keyPath, []byte("not-base64!"), 0o600))

	_, err := NewEncryptedStore(path, NewAESEncryptor("krci-test", dir, io.Discard), io.Discard).Load()
	require.ErrorContains(t, err, "decrypting token: getting encryption key: key file "+keyPath+": decoding key: ")
	require.NotErrorIs(t, err, ErrNoToken)
	assert.FileExists(t, path)
}

func TestLoadKeyringUnavailableReadsKeyFile(t *testing.T) {
	mockKeyring(t, errors.New("keyring locked"))

	dir := t.TempDir()
	path := saveTestToken(t, dir, true)

	var buf bytes.Buffer
	store := NewEncryptedStore(path, NewAESEncryptor("krci-test", dir, &buf), &buf)

	tok, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "kept", tok.AccessToken)
	assert.Contains(t, buf.String(), "Warning: OS keyring unavailable, using file-based key storage.\n")
}

func TestEncryptReusesKeyFile(t *testing.T) {
	tests := []struct {
		name       string
		keyringErr error
	}{
		{name: "empty keyring"},
		{name: "unavailable keyring", keyringErr: errors.New("keyring locked")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKeyring(t, tt.keyringErr)

			dir := t.TempDir()
			keyFile := encodeKey(newTestEncryptor().key)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".keyfile"), []byte(keyFile), 0o600))

			ciphertext, err := NewAESEncryptor("krci-test", dir, io.Discard).Encrypt([]byte("payload"))
			require.NoError(t, err)

			_, err = newTestEncryptor().Decrypt(ciphertext)
			require.NoError(t, err, "Encrypt did not use the .keyfile key")

			if tt.keyringErr == nil {
				_, err = keyring.Get("krci-test", keyringUser)
				require.ErrorIs(t, err, keyring.ErrNotFound, "Encrypt stored a new key in the keyring")
			}
		})
	}
}

func TestEncryptEmptyKeyringIgnoresBadKeyFile(t *testing.T) {
	for name, keyFile := range map[string]string{"malformed": "not-base64!", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			mockKeyring(t, nil)

			dir := t.TempDir()
			keyPath := filepath.Join(dir, ".keyfile")
			require.NoError(t, os.WriteFile(keyPath, []byte(keyFile), 0o600))

			var buf bytes.Buffer

			ciphertext, err := NewAESEncryptor("krci-test", dir, &buf).Encrypt([]byte("payload"))
			require.NoError(t, err)
			assert.Contains(t, buf.String(), "Warning: ignoring the key file and creating a new key: key file "+keyPath+": ")

			data, err := os.ReadFile(keyPath)
			require.NoError(t, err)
			assert.Equal(t, keyFile, string(data), "Encrypt changed .keyfile")

			_, err = keyring.Get("krci-test", keyringUser)
			require.NoError(t, err, "Encrypt did not store its key in the keyring")

			plaintext, err := NewAESEncryptor("krci-test", dir, io.Discard).Decrypt(ciphertext)
			require.NoError(t, err)
			assert.Equal(t, []byte("payload"), plaintext)
		})
	}
}

func TestEncryptBadKeyFileAndKeyringRejectsKey(t *testing.T) {
	// Get reports no key and Set fails, so the new key falls back to the bad .keyfile.
	mockKeyring(t, keyring.ErrNotFound)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, ".keyfile")
	require.NoError(t, os.WriteFile(keyPath, []byte("not-base64!"), 0o600))

	var buf bytes.Buffer

	_, err := NewAESEncryptor("krci-test", dir, &buf).Encrypt([]byte("payload"))
	require.ErrorContains(t, err, "getting encryption key: decoding existing key file "+keyPath+": ")
	assert.Equal(t, fmt.Sprintf("Warning: ignoring the key file and creating a new key: key file %s: "+
		"decoding key: illegal base64 data at input byte 3\n"+
		"Warning: OS keyring unavailable, using file-based key storage.\n"+
		"  Ensure %s is protected.\n", keyPath, keyPath), buf.String())

	data, readErr := os.ReadFile(keyPath)
	require.NoError(t, readErr)
	assert.Equal(t, "not-base64!", string(data), "Encrypt changed .keyfile")
}
