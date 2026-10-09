package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not parallel: the tests set HOME and KRCI_* through t.Setenv.

const warningPrefix = "Warning: error reading config file: "

// isolatedConfigDir points HOME and USERPROFILE at an empty directory and
// blanks every KRCI_* variable. It returns the config directory, which is not
// created.
func isolatedConfigDir(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "KRCI_") {
			t.Setenv(name, "")
		}
	}

	return filepath.Join(home, ".config", "krci")
}

func writeConfigFile(t *testing.T, dir, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600))
}

// newLoader builds a Loader over a fresh flag set and parses args after New.
func newLoader(t *testing.T, args []string, w io.Writer) *Loader {
	t.Helper()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	AddFlags(fs)

	l := New(fs, w)
	require.NoError(t, fs.Parse(args))

	return l
}

func resolve(t *testing.T, args []string) *Config {
	t.Helper()

	cfg, err := newLoader(t, args, io.Discard).Resolve()
	require.NoError(t, err)

	return cfg
}

func TestDefaults(t *testing.T) {
	isolatedConfigDir(t)

	cfg := resolve(t, nil)

	assert.Equal(t, "krci-cli", cfg.ClientID)
	assert.Equal(t, "openid email profile", cfg.Scopes)
	assert.Equal(t, "krci", cfg.KeyringService)
	assert.Empty(t, cfg.IssuerURL)
	assert.Empty(t, cfg.PortalURL)
	assert.Empty(t, cfg.ClusterName)
	assert.Empty(t, cfg.Namespace)
}

func TestEnvVarOverride(t *testing.T) {
	isolatedConfigDir(t)

	t.Setenv("KRCI_ISSUER_URL", "https://test-idp.example.com/realms/shared")
	t.Setenv("KRCI_CLIENT_ID", "custom-cli")
	t.Setenv("KRCI_CLUSTER_NAME", "env-cluster")

	cfg := resolve(t, nil)

	assert.Equal(t, "https://test-idp.example.com/realms/shared", cfg.IssuerURL)
	assert.Equal(t, "custom-cli", cfg.ClientID)
	assert.Equal(t, "env-cluster", cfg.ClusterName)
}

func TestFileValues(t *testing.T) {
	dir := isolatedConfigDir(t)

	writeConfigFile(t, dir, `issuer-url: https://file-idp.example.com/realms/shared
client-id: file-cli
scopes: openid
portal-url: https://file.example.com
cluster-name: file-cluster
namespace: file-ns
`)

	cfg := resolve(t, nil)

	assert.Equal(t, "https://file-idp.example.com/realms/shared", cfg.IssuerURL)
	assert.Equal(t, "file-cli", cfg.ClientID)
	assert.Equal(t, "openid", cfg.Scopes)
	assert.Equal(t, "https://file.example.com", cfg.PortalURL)
	assert.Equal(t, "file-cluster", cfg.ClusterName)
	assert.Equal(t, "file-ns", cfg.Namespace)
}

func TestPortalURLPrecedence(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		file string
		want string
	}{
		{
			name: "flag over env and file",
			args: []string{"--portal-url", "https://flag.example.com"},
			env:  "https://env.example.com",
			file: "https://file.example.com",
			want: "https://flag.example.com",
		},
		{
			name: "env over file",
			env:  "https://env.example.com",
			file: "https://file.example.com",
			want: "https://env.example.com",
		},
		{
			name: "file over default",
			file: "https://file.example.com",
			want: "https://file.example.com",
		},
		{
			name: "default",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := isolatedConfigDir(t)

			if tt.file != "" {
				writeConfigFile(t, dir, "portal-url: "+tt.file+"\n")
			}

			if tt.env != "" {
				t.Setenv("KRCI_PORTAL_URL", tt.env)
			}

			assert.Equal(t, tt.want, resolve(t, tt.args).PortalURL)
		})
	}
}

func TestEmptyEnvFallsThroughToFile(t *testing.T) {
	dir := isolatedConfigDir(t)

	writeConfigFile(t, dir, "portal-url: https://file.example.com\n")
	t.Setenv("KRCI_PORTAL_URL", "")

	assert.Equal(t, "https://file.example.com", resolve(t, nil).PortalURL)
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
			isolatedConfigDir(t)
			t.Setenv("KRCI_PORTAL_URL", tt.input)

			assert.Equal(t, tt.want, resolve(t, nil).PortalURL)
		})
	}
}

func TestResolvePaths(t *testing.T) {
	dir := isolatedConfigDir(t)

	cfg := resolve(t, nil)

	assert.Equal(t, dir, cfg.ConfigDir)
	assert.Equal(t, filepath.Join(dir, "tokens.enc"), cfg.TokenPath)
}

func TestResolvePathsWithoutHome(t *testing.T) {
	isolatedConfigDir(t)
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	cfg := resolve(t, nil)

	assert.Equal(t, filepath.Join(".config", "krci"), cfg.ConfigDir)
	assert.Equal(t, filepath.Join(".config", "krci", "tokens.enc"), cfg.TokenPath)
}

func TestBrokenConfigFileWarns(t *testing.T) {
	dir := isolatedConfigDir(t)

	writeConfigFile(t, dir, "portal-url: [broken\n")
	t.Setenv("KRCI_CLIENT_ID", "custom-cli")

	var w bytes.Buffer

	cfg, err := newLoader(t, nil, &w).Resolve()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(w.String(), warningPrefix), "warning = %q", w.String())
	assert.True(t, strings.HasSuffix(w.String(), "\n"), "warning = %q", w.String())
	assert.Equal(t, 1, strings.Count(w.String(), "\n"), "warning = %q", w.String())
	assert.Equal(t, "custom-cli", cfg.ClientID)
	assert.Equal(t, "openid email profile", cfg.Scopes)
	assert.Empty(t, cfg.PortalURL)
}

func TestMissingConfigFileIsSilent(t *testing.T) {
	isolatedConfigDir(t)

	var w bytes.Buffer

	newLoader(t, nil, &w)

	assert.Empty(t, w.String())
}

func TestSaveAndLoad(t *testing.T) {
	dir := isolatedConfigDir(t)

	cfg := &Config{
		IssuerURL:   "https://idp.example.com/realms/shared",
		ClientID:    "custom-client",
		Scopes:      "openid",
		PortalURL:   "https://portal.example.com",
		ClusterName: "saved-cluster",
		Namespace:   "saved-ns",
		ConfigDir:   dir,
	}

	require.NoError(t, Save(cfg))

	configPath := filepath.Join(dir, "config.yaml")
	info, err := os.Stat(configPath)
	require.NoError(t, err, "config file not created")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	got := resolve(t, nil)

	assert.Equal(t, cfg.IssuerURL, got.IssuerURL)
	assert.Equal(t, cfg.ClientID, got.ClientID)
	assert.Equal(t, cfg.Scopes, got.Scopes)
	assert.Equal(t, cfg.PortalURL, got.PortalURL)
	assert.Equal(t, cfg.ClusterName, got.ClusterName)
	assert.Equal(t, cfg.Namespace, got.Namespace)
}

func TestSaveTrimsPortalURLTrailingSlash(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		PortalURL: "https://portal.example.com/",
		ConfigDir: dir,
	}

	require.NoError(t, Save(cfg))

	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, err, "reading config file")

	assert.Contains(t, string(data), "portal-url: https://portal.example.com\n")
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

	assert.Contains(t, content, "issuer-url")
	assert.NotContains(t, content, "client-id", "default client-id should not be saved to config file")
	assert.NotContains(t, content, "scopes", "default scopes should not be saved to config file")
}
