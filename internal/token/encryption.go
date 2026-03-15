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

// Encryptor encrypts and decrypts token data using AES-256-GCM.
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

type aesEncryptor struct {
	keyringService string
	keyringUser    string
	configDir      string
	mu             sync.Mutex
	key            []byte
}

// NewAESEncryptor creates an AES-256-GCM encryptor.
// The encryption key is stored in the OS keyring with file fallback.
func NewAESEncryptor(keyringService, configDir string) Encryptor {
	return &aesEncryptor{
		keyringService: keyringService,
		keyringUser:    "encryption-key",
		configDir:      configDir,
	}
}

// gcm returns an initialized AES-256-GCM cipher, caching the key on first successful load.
// Uses a mutex instead of sync.Once so transient key-loading failures can be retried.
func (e *aesEncryptor) gcm() (cipher.AEAD, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.key == nil {
		key, err := getOrCreateKey(e.keyringService, e.keyringUser, e.configDir)
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

// Decrypt reverses Encrypt. Expects nonce || ciphertext || tag.
func (e *aesEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	gcm, err := e.gcm()
	if err != nil {
		return nil, err
	}

	minLen := gcm.NonceSize() + gcm.Overhead()
	if len(ciphertext) < minLen {
		return nil, errors.New("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting data: %w", err)
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
