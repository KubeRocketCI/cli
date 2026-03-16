// Package config provides configuration loading for the krci CLI.
// It uses Viper for layered config: flags > env vars > config file > defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all resolved configuration values.
type Config struct {
	IssuerURL      string `mapstructure:"issuer-url"`
	ClientID       string `mapstructure:"client-id"`
	Scopes         string `mapstructure:"scopes"`
	PortalURL      string `mapstructure:"portal-url"`
	Namespace      string `mapstructure:"namespace"`
	APIServer      string `mapstructure:"api-server"`
	CAData         string `mapstructure:"ca-data"`
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

// Init sets up Viper defaults and reads the config file.
// Call this early — before Cobra parses flags.
func Init() {
	configDir := DefaultConfigDir()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir)

	viper.SetEnvPrefix("KRCI")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	viper.SetDefault("issuer-url", "")
	viper.SetDefault("client-id", "krci-cli")
	viper.SetDefault("scopes", "openid email profile")
	viper.SetDefault("portal-url", "")
	viper.SetDefault("namespace", "")
	viper.SetDefault("api-server", "")
	viper.SetDefault("ca-data", "")

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			fmt.Fprintf(os.Stderr, "Warning: error reading config file: %v\n", err)
		}
	}
}

// BindFlags registers persistent flags on the root command and binds them to Viper.
func BindFlags(cmd *cobra.Command) {
	pf := cmd.PersistentFlags()
	pf.String("issuer-url", "", "OIDC issuer URL (Keycloak realm)")
	pf.String("client-id", "krci-cli", "OIDC client ID")
	pf.String("portal-url", "", "KubeRocketCI Portal URL")
	pf.StringP("namespace", "n", "", "Kubernetes namespace")
	pf.String("api-server", "", "Kubernetes API server URL")
	pf.String("ca-data", "", "Base64-encoded Kubernetes CA certificate")

	_ = viper.BindPFlags(pf)
}

// Resolve reads the merged config AFTER Cobra has parsed flags.
// Call this from PersistentPreRunE so flags are available.
func Resolve() (*Config, error) {
	configDir := DefaultConfigDir()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.ConfigDir = configDir
	cfg.TokenPath = filepath.Join(configDir, "tokens.enc")
	cfg.KeyringService = "krci"

	return &cfg, nil
}

// Save persists user-provided values to the config file.
// Only writes non-empty values so defaults and env vars are not baked in.
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
		existing.Set("portal-url", cfg.PortalURL)
	}
	if cfg.Namespace != "" {
		existing.Set("namespace", cfg.Namespace)
	}
	if cfg.Scopes != "" && cfg.Scopes != "openid email profile" {
		existing.Set("scopes", cfg.Scopes)
	}
	if cfg.APIServer != "" {
		existing.Set("api-server", cfg.APIServer)
	}
	if cfg.CAData != "" {
		existing.Set("ca-data", cfg.CAData)
	}

	if err := existing.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return os.Chmod(configPath, 0600)
}
