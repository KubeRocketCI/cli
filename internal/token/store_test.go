package token

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	store := NewEncryptedStore(path, newTestEncryptor())

	tok := &StoredToken{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		IDToken:      "id-789",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
		IssuerURL:    "https://idp.example.com/realms/shared",
		ClientID:     "krci-cli",
	}

	require.NoError(t, store.Save(tok))

	// Verify file exists with correct permissions.
	info, err := os.Stat(path)
	require.NoError(t, err, "token file not created")
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := store.Load()
	require.NoError(t, err)

	assert.Equal(t, tok.AccessToken, loaded.AccessToken)
	assert.Equal(t, tok.RefreshToken, loaded.RefreshToken)
	assert.Equal(t, tok.IDToken, loaded.IDToken)
	assert.True(t, loaded.ExpiresAt.Equal(tok.ExpiresAt), "ExpiresAt = %v, want %v", loaded.ExpiresAt, tok.ExpiresAt)
	assert.Equal(t, tok.IssuerURL, loaded.IssuerURL)
	assert.Equal(t, tok.ClientID, loaded.ClientID)
}

func TestStoreLoadNonExistent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.enc")
	store := NewEncryptedStore(path, newTestEncryptor())

	_, err := store.Load()
	assert.ErrorIs(t, err, ErrNoToken)
}

func TestStoreLoadCorruptedAutoCleanup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	store := NewEncryptedStore(path, newTestEncryptor())

	// Write garbage data.
	require.NoError(t, os.WriteFile(path, []byte("corrupted-garbage-data-that-is-long-enough"), 0600))

	_, err := store.Load()
	assert.ErrorIs(t, err, ErrNoToken, "auto-cleanup should return ErrNoToken")

	// Verify corrupted file was removed.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "corrupted token file was not removed")
}

func TestStoreClear(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	store := NewEncryptedStore(path, newTestEncryptor())

	// Save then clear.
	tok := &StoredToken{AccessToken: "test", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, store.Save(tok))
	require.NoError(t, store.Clear())

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "token file still exists after Clear()")
}

func TestStoreClearNonExistent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.enc")
	store := NewEncryptedStore(path, newTestEncryptor())

	// Clear on non-existent file should not error.
	require.NoError(t, store.Clear())
}

func TestStoredTokenValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expiry time.Time
		want   bool
	}{
		{"future", time.Now().Add(time.Hour), true},
		{"almost expired", time.Now().Add(31 * time.Second), true},
		{"within buffer", time.Now().Add(29 * time.Second), false},
		{"expired", time.Now().Add(-time.Minute), false},
		{"zero time", time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tok := &StoredToken{ExpiresAt: tt.expiry}
			assert.Equal(t, tt.want, tok.Valid())
		})
	}
}
