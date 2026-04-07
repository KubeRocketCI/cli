package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests cannot use t.Parallel() because they share the global
// viper singleton (viper.Reset / Init / Resolve) and use t.Setenv.

func TestDefaults(t *testing.T) {
	viper.Reset()
	Init()

	cfg, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, "krci-cli", cfg.ClientID)
	assert.Equal(t, "openid email profile", cfg.Scopes)
	assert.Equal(t, "krci", cfg.KeyringService)
}

func TestEnvVarOverride(t *testing.T) {
	viper.Reset()

	t.Setenv("KRCI_ISSUER_URL", "https://test-idp.example.com/realms/shared")
	t.Setenv("KRCI_CLIENT_ID", "custom-cli")

	Init()

	cfg, err := Resolve()
	require.NoError(t, err)

	assert.Equal(t, "https://test-idp.example.com/realms/shared", cfg.IssuerURL)
	assert.Equal(t, "custom-cli", cfg.ClientID)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		IssuerURL: "https://idp.example.com/realms/shared",
		PortalURL: "https://portal.example.com",
		ClientID:  "custom-client",
		ConfigDir: dir,
	}

	require.NoError(t, Save(cfg))

	// Verify file exists with correct permissions.
	configPath := filepath.Join(dir, "config.yaml")
	info, err := os.Stat(configPath)
	require.NoError(t, err, "config file not created")
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Load it back via Viper.
	viper.Reset()
	viper.SetConfigFile(configPath)
	require.NoError(t, viper.ReadInConfig())

	assert.Equal(t, cfg.IssuerURL, viper.GetString("issuer-url"))
	assert.Equal(t, cfg.PortalURL, viper.GetString("portal-url"))
}

func TestResolveTrimsPortalURLTrailingSlashes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no trailing slash", "https://portal.example.com", "https://portal.example.com"},
		{"single trailing slash", "https://portal.example.com/", "https://portal.example.com"},
		{"multiple trailing slashes", "https://portal.example.com///", "https://portal.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Setenv("KRCI_PORTAL_URL", tt.input)
			Init()

			cfg, err := Resolve()
			require.NoError(t, err)

			assert.Equal(t, tt.want, cfg.PortalURL)
		})
	}
}

func TestSaveTrimsPortalURLTrailingSlash(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		PortalURL: "https://portal.example.com/",
		ConfigDir: dir,
	}

	require.NoError(t, Save(cfg))

	configPath := filepath.Join(dir, "config.yaml")
	viper.Reset()
	viper.SetConfigFile(configPath)
	require.NoError(t, viper.ReadInConfig())

	assert.Equal(t, "https://portal.example.com", viper.GetString("portal-url"))
}

func TestSaveSkipsDefaults(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		IssuerURL: "https://idp.example.com",
		ClientID:  "krci-cli",             // default -- should not be saved
		Scopes:    "openid email profile", // default -- should not be saved
		ConfigDir: dir,
	}

	require.NoError(t, Save(cfg))

	configPath := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err, "reading config file")

	content := string(data)

	// Default values should not be persisted.
	assert.False(t, strings.Contains(content, "client-id"), "default client-id should not be saved to config file")
	assert.False(t, strings.Contains(content, "scopes"), "default scopes should not be saved to config file")
}
