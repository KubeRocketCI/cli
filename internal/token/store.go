package token

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

// ErrNoToken indicates no stored token exists.
var ErrNoToken = errors.New("no stored token")

// Store persists and retrieves encrypted OAuth tokens.
type Store interface {
	Save(tok *StoredToken) error
	Load() (*StoredToken, error)
	Clear() error
}

// StoredToken holds the persisted OAuth token data.
// StoredToken holds the persisted OAuth token data.
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	IssuerURL    string    `json:"issuer_url"`
	ClientID     string    `json:"client_id"`
}

// NewStoredToken creates a StoredToken from an oauth2.Token and metadata.
func NewStoredToken(tok *oauth2.Token, issuerURL, clientID string) *StoredToken {
	rawIDToken, _ := tok.Extra("id_token").(string)

	return &StoredToken{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		IDToken:      rawIDToken,
		ExpiresAt:    tok.Expiry,
		IssuerURL:    issuerURL,
		ClientID:     clientID,
	}
}

// ToOAuth2Token converts to oauth2.Token for use with oauth2.TokenSource.
func (t *StoredToken) ToOAuth2Token() *oauth2.Token {
	tok := &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		Expiry:       t.ExpiresAt,
		TokenType:    "Bearer",
	}

	return tok.WithExtra(map[string]any{
		"id_token": t.IDToken,
	})
}

// Valid returns true if the access token has not expired (with 30s buffer).
func (t *StoredToken) Valid() bool {
	return time.Until(t.ExpiresAt) > 30*time.Second
}

// EncryptedFileStore persists tokens as AES-256-GCM encrypted JSON files.
type EncryptedFileStore struct {
	path      string
	encryptor Encryptor
}

// NewEncryptedStore creates a token store that encrypts data at rest.
func NewEncryptedStore(path string, enc Encryptor) *EncryptedFileStore {
	return &EncryptedFileStore{
		path:      path,
		encryptor: enc,
	}
}

// Save encrypts and writes the token to disk atomically (temp file + fsync + rename).
func (s *EncryptedFileStore) Save(tok *StoredToken) error {
	plaintext, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}

	ciphertext, err := s.encryptor.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating token dir: %w", err)
	}

	return atomicWrite(s.path, ciphertext, 0600)
}

// Load reads and decrypts the token from disk.
func (s *EncryptedFileStore) Load() (*StoredToken, error) {
	ciphertext, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoToken
		}

		return nil, fmt.Errorf("reading token file: %w", err)
	}

	plaintext, err := s.encryptor.Decrypt(ciphertext)
	if err != nil {
		// Remove corrupted/undecryptable token file so user can re-authenticate cleanly.
		fmt.Fprintf(os.Stderr, "Warning: removing undecryptable token file (%s): %v\n", s.path, err)
		fmt.Fprintf(os.Stderr, "  Run 'krci auth login' to re-authenticate.\n")

		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Warning: could not remove corrupted file: %v\n", removeErr)
		}

		return nil, ErrNoToken
	}

	var tok StoredToken
	if err := json.Unmarshal(plaintext, &tok); err != nil {
		return nil, fmt.Errorf("unmarshaling token: %w", err)
	}

	return &tok, nil
}

// Clear removes the stored token file.
func (s *EncryptedFileStore) Clear() error {
	err := os.Remove(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing token file: %w", err)
	}

	return nil
}

// atomicWrite writes data to a temp file, fsyncs, then renames atomically.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".tokens-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	tmpName := tmp.Name()
	success := false

	defer func() {
		if !success {
			_ = tmp.Close()        // best-effort cleanup
			_ = os.Remove(tmpName) // best-effort cleanup
		}
	}()

	// Set restrictive permissions before writing sensitive data to avoid
	// a window where the file is readable by other users on shared systems.
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("setting token file permissions: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing token file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing token file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing token file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming token file: %w", err)
	}

	success = true

	return nil
}
