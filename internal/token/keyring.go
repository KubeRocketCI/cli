package token

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
)

// ErrKeyringAccess indicates the OS keyring is not available.
var ErrKeyringAccess = errors.New("unable to access OS keyring")

// configPath safely resolves a filename within the config directory.
// Returns an error if the resolved path escapes the config directory.
func configPath(configDir, filename string) (string, error) {
	cleanDir, err := filepath.Abs(filepath.Clean(configDir))
	if err != nil {
		return "", fmt.Errorf("resolving config dir: %w", err)
	}

	resolved := filepath.Join(cleanDir, filename)

	if !strings.HasPrefix(resolved, cleanDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes config directory", filename)
	}

	return resolved, nil
}

// ensureConfigDir creates the config directory with 0700 permissions.
func ensureConfigDir(configDir string) (string, error) {
	cleanDir, err := filepath.Abs(filepath.Clean(configDir))
	if err != nil {
		return "", fmt.Errorf("resolving config dir: %w", err)
	}

	if err := os.MkdirAll(cleanDir, 0o700); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}

	return cleanDir, nil
}

// getOrCreateKey retrieves the 256-bit AES key using the configured backend.
// Respects KRCI_KEYRING_BACKEND env var: "keyring" (default) or "file".
func getOrCreateKey(service, user, configDir string) ([]byte, error) {
	backend := os.Getenv("KRCI_KEYRING_BACKEND")
	if backend == "file" {
		return getOrCreateFileKey(configDir)
	}

	return getOrCreateKeyringKey(service, user, configDir)
}

// getOrCreateKeyringKey tries the OS keyring, falling back to file if unavailable.
func getOrCreateKeyringKey(service, user, configDir string) ([]byte, error) {
	encoded, err := keyring.Get(service, user)
	if err == nil {
		return decodeKey(encoded)
	}

	if !errors.Is(err, keyring.ErrNotFound) {
		warnKeyringFallback(configDir)

		return getOrCreateFileKey(configDir)
	}

	key, err := generateKey()
	if err != nil {
		return nil, err
	}

	if err := keyring.Set(service, user, encodeKey(key)); err != nil {
		warnKeyringFallback(configDir)

		return key, saveKeyToFile(key, configDir)
	}

	return key, nil
}

// warnKeyringFallback prints a warning when falling back to file-based key storage.
func warnKeyringFallback(configDir string) {
	keyPath, err := configPath(configDir, ".keyfile")
	if err != nil {
		return
	}

	fmt.Fprintf(os.Stderr, "Warning: OS keyring unavailable, using file-based key storage.\n")
	fmt.Fprintf(os.Stderr, "  Ensure %s is protected.\n", keyPath)
}

// generateKey creates a cryptographically secure 256-bit key.
func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating encryption key: %w", err)
	}

	return key, nil
}

// getOrCreateFileKey loads the key from file, or generates and saves one.
func getOrCreateFileKey(configDir string) ([]byte, error) {
	keyPath, err := configPath(configDir, ".keyfile")
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(keyPath)
	if err == nil {
		validateFilePermissions(keyPath)

		return decodeKey(string(data))
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading key file: %w", err)
	}

	key, err := generateKey()
	if err != nil {
		return nil, err
	}

	return key, saveKeyToFile(key, configDir)
}

// saveKeyToFile writes the key atomically with exclusive create and fsync.
func saveKeyToFile(key []byte, configDir string) error {
	cleanDir, err := ensureConfigDir(configDir)
	if err != nil {
		return err
	}

	keyPath, err := configPath(cleanDir, ".keyfile")
	if err != nil {
		return err
	}

	encoded := []byte(encodeKey(key))

	// Exclusive create prevents race conditions between concurrent CLI processes.
	f, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return readWinnersKey(keyPath, key)
	}

	if err != nil {
		return fmt.Errorf("creating key file: %w", err)
	}

	writeOK := false

	defer func() {
		if !writeOK {
			_ = f.Close()
			_ = os.Remove(keyPath)
		}
	}()

	if _, err = f.Write(encoded); err != nil {
		return fmt.Errorf("writing key file: %w", err)
	}

	if err = f.Sync(); err != nil {
		return fmt.Errorf("syncing key file: %w", err)
	}

	writeOK = true

	return f.Close()
}

// readWinnersKey reads the key written by a concurrent process that won the race.
func readWinnersKey(keyPath string, dst []byte) error {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("reading existing key file: %w", err)
	}

	existingKey, err := decodeKey(string(data))
	if err != nil {
		return fmt.Errorf("decoding existing key: %w", err)
	}

	copy(dst, existingKey)

	return nil
}

// validateFilePermissions warns if the key file is readable by others (Unix only).
func validateFilePermissions(path string) {
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s has permissions %04o, expected 0600.\n", path, mode)
		fmt.Fprintf(os.Stderr, "  Run: chmod 600 %s\n", path)
	}
}
