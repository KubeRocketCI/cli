// Package config provides configuration loading for the krci CLI.
// It uses Viper for layered config: flags > env vars > config file > defaults.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config holds all resolved configuration values.
type Config struct {
	IssuerURL      string `mapstructure:"issuer-url"`
	ClientID       string `mapstructure:"client-id"`
	Scopes         string `mapstructure:"scopes"`
	PortalURL      string `mapstructure:"portal-url"`
	ClusterName    string `mapstructure:"cluster-name"`
	Namespace      string `mapstructure:"namespace"`
	TokenPath      string
	KeyringService string
	ConfigDir      string
}

// DefaultConfigDir returns ~/.config/krci.
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "krci")
	}
	return filepath.Join(home, ".config", "krci")
}

// Loader resolves a Config from flags, env vars, the config file, and defaults.
type Loader struct {
	v         *viper.Viper
	configDir string
}

// AddFlags registers the configuration flags on fs.
func AddFlags(fs *pflag.FlagSet) {
	fs.String("portal-url", "", "KubeRocketCI Portal URL")
}

// New builds a Loader over fs: flags > env > file > defaults. Register the
// flags on fs before calling New; flags added later are not bound. New reads
// the config file once; a file that exists but cannot be read or parsed is
// reported on w and ignored. Flag values are read when Resolve runs. w must be
// non-nil.
func New(fs *pflag.FlagSet, w io.Writer) *Loader {
	configDir := DefaultConfigDir()

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	v.SetEnvPrefix("KRCI")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	v.SetDefault("issuer-url", "")
	v.SetDefault("client-id", "krci-cli")
	v.SetDefault("scopes", "openid email profile")
	v.SetDefault("portal-url", "")
	v.SetDefault("cluster-name", "")
	v.SetDefault("namespace", "")

	_ = v.BindPFlags(fs)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			_, _ = fmt.Fprintf(w, "Warning: error reading config file: %v\n", err)
		}
	}

	return &Loader{v: v, configDir: configDir}
}

// Resolve returns the merged configuration. Call it after the flags are parsed.
func (l *Loader) Resolve() (*Config, error) {
	var cfg Config
	if err := l.v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.PortalURL = strings.TrimRight(cfg.PortalURL, "/")
	cfg.ConfigDir = l.configDir
	cfg.TokenPath = filepath.Join(l.configDir, "tokens.enc")
	cfg.KeyringService = "krci"

	return &cfg, nil
}

// Save persists resolved configuration values to the config file.
// Non-empty values (including those resolved from env vars) are written
// so they survive across sessions without requiring env vars again.
func Save(cfg *Config) error {
	configDir := cfg.ConfigDir
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	existing := viper.New()
	existing.SetConfigFile(configPath)
	_ = existing.ReadInConfig() // OK if doesn't exist yet

	if cfg.IssuerURL != "" {
		existing.Set("issuer-url", cfg.IssuerURL)
	}
	if cfg.ClientID != "" && cfg.ClientID != "krci-cli" {
		existing.Set("client-id", cfg.ClientID)
	}
	if cfg.PortalURL != "" {
		existing.Set("portal-url", strings.TrimRight(cfg.PortalURL, "/"))
	}
	if cfg.ClusterName != "" {
		existing.Set("cluster-name", cfg.ClusterName)
	}
	if cfg.Namespace != "" {
		existing.Set("namespace", cfg.Namespace)
	}
	if cfg.Scopes != "" && cfg.Scopes != "openid email profile" {
		existing.Set("scopes", cfg.Scopes)
	}

	if err := existing.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return os.Chmod(configPath, 0600)
}
