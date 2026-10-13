// Package token provides encrypted token persistence for the krci CLI.
package token

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrInvalidCiphertext indicates data shorter than a nonce and tag, or data
// that does not decrypt under a loaded key.
var ErrInvalidCiphertext = errors.New("invalid ciphertext")

// minCiphertextLen is the AES-GCM standard nonce (12 bytes) plus tag (16 bytes).
const minCiphertextLen = 12 + 16

// Encryptor encrypts and decrypts token data using AES-256-GCM.
// Decrypt wraps ErrInvalidCiphertext only when the key loaded and the data
// does not decrypt under it; a key failure does not wrap it.
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

type aesEncryptor struct {
	keyringService string
	keyringUser    string
	configDir      string
	w              io.Writer
	mu             sync.Mutex
	key            []byte
}

// NewAESEncryptor creates an AES-256-GCM encryptor.
// The encryption key is stored in the OS keyring with file fallback.
// Key-storage warnings are written to w, which must be non-nil.
// Panics if w is nil.
func NewAESEncryptor(keyringService, configDir string, w io.Writer) Encryptor {
	if w == nil {
		panic("token.NewAESEncryptor: nil writer")
	}

	return &aesEncryptor{
		keyringService: keyringService,
		keyringUser:    "encryption-key",
		configDir:      configDir,
		w:              w,
	}
}

// gcm returns an initialized AES-256-GCM cipher, caching the key on first successful load.
// Uses a mutex instead of sync.Once so transient key-loading failures can be retried.
func (e *aesEncryptor) gcm() (cipher.AEAD, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.key == nil {
		key, err := getOrCreateKey(e.keyringService, e.keyringUser, e.configDir, e.w)
		if err != nil {
			return nil, fmt.Errorf("getting encryption key: %w", err)
		}
		e.key = key
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	return cipher.NewGCM(block)
}

// Encrypt uses AES-256-GCM. Output: nonce (12 bytes) || ciphertext || GCM tag (16 bytes).
func (e *aesEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	gcm, err := e.gcm()
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce, so result is: nonce || ciphertext || tag.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt. Expects nonce || ciphertext || tag. Data too short
// to hold a nonce and tag fails before the key is loaded.
func (e *aesEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < minCiphertextLen {
		return nil, fmt.Errorf("%w: ciphertext too short", ErrInvalidCiphertext)
	}

	gcm, err := e.gcm()
	if err != nil {
		return nil, err
	}

	nonce, encrypted := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCiphertext, err)
	}

	return plaintext, nil
}

// encodeKey encodes a raw key as base64 for storage.
func encodeKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

// decodeKey decodes a base64-encoded key and validates its length.
func decodeKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key has invalid length %d, expected 32", len(key))
	}
	return key, nil
}
