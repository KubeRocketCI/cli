package token

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEncryptor uses a fixed key (bypasses keyring in tests).
type testEncryptor struct {
	key []byte
}

func newTestEncryptor() *testEncryptor {
	return &testEncryptor{key: bytes.Repeat([]byte("k"), 32)}
}

func (e *testEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	real := &aesEncryptor{key: e.key}
	return real.Encrypt(plaintext)
}

func (e *testEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	real := &aesEncryptor{key: e.key}
	return real.Decrypt(ciphertext)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()

	enc := newTestEncryptor()

	original := []byte(`{"access_token":"abc123","refresh_token":"xyz789"}`)

	encrypted, err := enc.Encrypt(original)
	require.NoError(t, err)
	assert.NotEqual(t, original, encrypted, "ciphertext equals plaintext")

	decrypted, err := enc.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, original, decrypted)
}

func TestDecryptErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ciphertext func(t *testing.T) []byte
	}{
		{
			name: "tampered data",
			ciphertext: func(t *testing.T) []byte {
				t.Helper()
				enc := newTestEncryptor()
				encrypted, err := enc.Encrypt([]byte("secret data"))
				require.NoError(t, err)
				// Flip a byte in the ciphertext (after the nonce).
				encrypted[15] ^= 0xff
				return encrypted
			},
		},
		{
			name: "ciphertext too short",
			ciphertext: func(_ *testing.T) []byte {
				return []byte("short")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enc := newTestEncryptor()
			_, err := enc.Decrypt(tt.ciphertext(t))
			require.Error(t, err)
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	t.Parallel()

	enc := newTestEncryptor()
	plaintext := []byte("same input")

	ct1, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	ct2, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	assert.NotEqual(t, ct1, ct2, "two encryptions of same plaintext produced identical ciphertext (nonce reuse)")
}

func TestEncodeDecodeKeyRoundTrip(t *testing.T) {
	t.Parallel()

	original := bytes.Repeat([]byte("a"), 32)

	encoded := encodeKey(original)
	decoded, err := decodeKey(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeKeyInvalidLength(t *testing.T) {
	t.Parallel()

	encoded := encodeKey([]byte("too-short"))

	_, err := decodeKey(encoded)
	require.Error(t, err)
}
