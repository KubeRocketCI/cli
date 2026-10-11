package token

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	store := NewEncryptedStore(path, newTestEncryptor(), io.Discard)

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
	store := NewEncryptedStore(path, newTestEncryptor(), io.Discard)

	_, err := store.Load()
	assert.ErrorIs(t, err, ErrNoToken)
}

func TestStoreLoadCorruptedAutoCleanup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	store := NewEncryptedStore(path, newTestEncryptor(), io.Discard)

	// Write garbage data.
	require.NoError(t, os.WriteFile(path, []byte("corrupted-garbage-data-that-is-long-enough"), 0600))

	_, err := store.Load()
	assert.ErrorIs(t, err, ErrNoToken, "auto-cleanup should return ErrNoToken")

	// Verify corrupted file was removed.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "corrupted token file was not removed")
}

// stubEncryptor fails every Decrypt with decryptErr, after running onDecrypt.
type stubEncryptor struct {
	decryptErr error
	onDecrypt  func()
}

func (s *stubEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return plaintext, nil
}

func (s *stubEncryptor) Decrypt([]byte) ([]byte, error) {
	if s.onDecrypt != nil {
		s.onDecrypt()
	}

	return nil, s.decryptErr
}

func TestStoreLoadUndecryptableWarning(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tokens.enc")
	require.NoError(t, os.WriteFile(path, []byte("ciphertext"), 0600))

	var buf bytes.Buffer
	store := NewEncryptedStore(path, &stubEncryptor{decryptErr: errors.New("decrypt failed")}, &buf)

	_, err := store.Load()
	require.ErrorIs(t, err, ErrNoToken)

	want := fmt.Sprintf("Warning: removing undecryptable token file (%s): decrypt failed\n"+
		"  Run 'krci auth login' to re-authenticate.\n", path)
	assert.Equal(t, want, buf.String())

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "undecryptable token file was not removed")
}

func TestStoreLoadRemoveFailureWarning(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tokens.enc")
	require.NoError(t, os.WriteFile(path, []byte("ciphertext"), 0600))

	// Replaces the file with a non-empty directory after it was read, so the removal fails.
	enc := &stubEncryptor{
		decryptErr: errors.New("decrypt failed"),
		onDecrypt: func() {
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(path, "child"), nil, 0600))
		},
	}

	var buf bytes.Buffer
	store := NewEncryptedStore(path, enc, &buf)

	_, err := store.Load()
	require.ErrorIs(t, err, ErrNoToken)

	removeErr := os.Remove(path)
	require.Error(t, removeErr)

	want := fmt.Sprintf("Warning: removing undecryptable token file (%s): decrypt failed\n"+
		"  Run 'krci auth login' to re-authenticate.\n"+
		"Warning: could not remove corrupted file: %v\n", path, removeErr)
	assert.Equal(t, want, buf.String())
}

// syncBuffer is a bytes.Buffer safe for concurrent writers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func TestStoreLoadConcurrentCorruptFileWarnsOnce(t *testing.T) {
	t.Parallel()

	const callers = 8

	path := filepath.Join(t.TempDir(), "tokens.enc")
	require.NoError(t, os.WriteFile(path, []byte("ciphertext"), 0600))

	// The delay keeps every unserialised caller inside Decrypt until all of them have read the file.
	enc := &stubEncryptor{
		decryptErr: errors.New("decrypt failed"),
		onDecrypt:  func() { time.Sleep(20 * time.Millisecond) },
	}

	var out syncBuffer
	store := NewEncryptedStore(path, enc, &out)

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, callers)
	)

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			_, errs[i] = store.Load()
		}()
	}

	close(start)
	wg.Wait()

	for i, err := range errs {
		assert.ErrorIs(t, err, ErrNoToken, "caller %d", i)
	}

	assert.Equal(t, 1, strings.Count(out.String(), "removing undecryptable token file"), out.String())
	assert.Equal(t, 1, strings.Count(out.String(), "Run 'krci auth login'"), out.String())

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "undecryptable token file was not removed")
}

func TestStoreSaveWaitsForInFlightLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tokens.enc")
	require.NoError(t, os.WriteFile(path, []byte("ciphertext"), 0600))

	var (
		entered = make(chan struct{})
		release = make(chan struct{})
	)

	enc := &stubEncryptor{
		decryptErr: errors.New("decrypt failed"),
		onDecrypt: func() {
			close(entered)
			<-release
		},
	}
	store := NewEncryptedStore(path, enc, io.Discard)

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		_, _ = store.Load()
	}()

	<-entered

	saveErr := make(chan error, 1)

	go func() {
		defer wg.Done()

		saveErr <- store.Save(&StoredToken{AccessToken: "fresh"})
	}()

	// A Save that is not serialised completes here and is then removed by the Load.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	require.NoError(t, <-saveErr)

	data, err := os.ReadFile(path)
	require.NoError(t, err, "Save result was removed by the in-flight Load")
	assert.Contains(t, string(data), "fresh")
}

func TestNewEncryptedStoreNilWriterPanics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "token.NewEncryptedStore: nil writer", func() {
		NewEncryptedStore(filepath.Join(t.TempDir(), "tokens.enc"), newTestEncryptor(), nil)
	})
}

func TestStoreClear(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.enc")
	store := NewEncryptedStore(path, newTestEncryptor(), io.Discard)

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
	store := NewEncryptedStore(path, newTestEncryptor(), io.Discard)

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
