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

// usesFileBackend reports whether KRCI_KEYRING_BACKEND selects the file
// backend. Any other value selects the keyring backend.
func usesFileBackend() bool {
	return os.Getenv("KRCI_KEYRING_BACKEND") == "file"
}

// loadKey returns the stored 256-bit AES key without creating one.
// File backend: .keyfile. Keyring backend: the OS keyring, then .keyfile.
// ErrKeyNotFound means none of the checked stores holds a key;
// ErrKeyringAccess means the keyring failed and .keyfile is absent. Warnings
// are written to w.
func loadKey(service, configDir string, w io.Writer) ([]byte, error) {
	if usesFileBackend() {
		return loadFileKey(configDir, w)
	}

	encoded, err := keyring.Get(service, keyringUser)
	if err == nil {
		return decodeKey(encoded)
	}

	if errors.Is(err, keyring.ErrNotFound) {
		return loadFileKey(configDir, w)
	}

	key, fileErr := loadFileKey(configDir, w)
	if errors.Is(fileErr, ErrKeyNotFound) {
		return nil, fmt.Errorf("%w: %w", ErrKeyringAccess, err)
	}

	if fileErr == nil {
		warnKeyringFallback(configDir, w)
	}

	return key, fileErr
}

// getOrCreateKey returns the key loadKey would find, or generates and stores
// one. File backend: .keyfile. Keyring backend: a readable .keyfile is reused
// while the keyring is empty, otherwise a new key goes to the keyring; when
// the keyring fails, .keyfile is read or created. Warnings are written to w.
func getOrCreateKey(service, configDir string, w io.Writer) ([]byte, error) {
	if usesFileBackend() {
		return loadOrCreateFileKey(configDir, w)
	}

	encoded, err := keyring.Get(service, keyringUser)
	if err == nil {
		return decodeKey(encoded)
	}

	if !errors.Is(err, keyring.ErrNotFound) {
		warnKeyringFallback(configDir, w)

		return loadOrCreateFileKey(configDir, w)
	}

	key, fileErr := loadFileKey(configDir, w)
	if fileErr == nil {
		return key, nil
	}

	if !errors.Is(fileErr, ErrKeyNotFound) {
		_, _ = fmt.Fprintf(w, "Warning: ignoring the key file and creating a new key: %v\n", fileErr)
	}

	return createKeyringKey(service, configDir, w)
}

// createKeyringKey generates a key and stores it in the keyring, or in
// .keyfile when the keyring rejects it.
func createKeyringKey(service, configDir string, w io.Writer) ([]byte, error) {
	key, err := generateKey()
	if err != nil {
		return nil, err
	}

	if err := keyring.Set(service, keyringUser, encodeKey(key)); err != nil {
		warnKeyringFallback(configDir, w)

		return key, saveKeyToFile(key, configDir)
	}

	return key, nil
}

// loadOrCreateFileKey reads .keyfile, or generates a key and stores it there
// when the file does not exist. A key file written concurrently by another
// process wins.
func loadOrCreateFileKey(configDir string, w io.Writer) ([]byte, error) {
	key, err := loadFileKey(configDir, w)
	if !errors.Is(err, ErrKeyNotFound) {
		return key, err
	}

	key, err = generateKey()
	if err != nil {
		return nil, err
	}

	return key, saveKeyToFile(key, configDir)
}

// warnKeyringFallback writes a warning to w when falling back to file-based key storage.
func warnKeyringFallback(configDir string, w io.Writer) {
	keyPath, err := configPath(configDir, ".keyfile")
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(w, "Warning: OS keyring unavailable, using file-based key storage.\n")
	_, _ = fmt.Fprintf(w, "  Ensure %s is protected.\n", keyPath)
}

// generateKey creates a cryptographically secure 256-bit key.
func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating encryption key: %w", err)
	}

	return key, nil
}

// loadFileKey reads the key from .keyfile. ErrKeyNotFound means the file does
// not exist.
func loadFileKey(configDir string, w io.Writer) ([]byte, error) {
	keyPath, err := configPath(configDir, ".keyfile")
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrKeyNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}

	validateFilePermissions(keyPath, w)

	key, err := decodeKey(string(data))
	if err != nil {
		return nil, fmt.Errorf("key file %s: %w", keyPath, err)
	}

	return key, nil
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
		return fmt.Errorf("decoding existing key file %s: %w", keyPath, err)
	}

	copy(dst, existingKey)

	return nil
}

// validateFilePermissions warns on w if the key file is readable by others (Unix only).
func validateFilePermissions(path string, w io.Writer) {
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		_, _ = fmt.Fprintf(w, "Warning: %s has permissions %04o, expected 0600.\n", path, mode)
		_, _ = fmt.Fprintf(w, "  Run: chmod 600 %s\n", path)
	}
}
