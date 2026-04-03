package portal

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds public configuration returned by the Portal.
type Config struct {
	ClusterName      string `json:"clusterName"`
	DefaultNamespace string `json:"defaultNamespace"`
	OIDCIssuerURL    string `json:"oidcIssuerUrl"`
}

// FetchConfig calls the Portal public config.get endpoint (no auth required)
// and returns the cluster configuration.
// The portal URL must use HTTPS.
func FetchConfig(portalURL string) (*Config, error) {
	u, err := url.Parse(portalURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("%w with a valid host: %q", ErrHTTPSRequired, portalURL)
	}

	return fetchConfig(portalURL)
}

func fetchConfig(portalURL string) (*Config, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(strings.TrimSuffix(portalURL, "/") + "/api/config.get")
	if err != nil {
		return nil, fmt.Errorf("requesting portal config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("portal config returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading portal config response: %w", err)
	}

	return parseConfig(body)
}

func parseConfig(body []byte) (*Config, error) {
	var cfg Config
	if err := parseTRPCResult(body, &cfg); err != nil {
		return nil, fmt.Errorf("parsing portal config: %w", err)
	}

	if !isValidConfig(&cfg) {
		return nil, fmt.Errorf("unexpected portal config response format")
	}

	return &cfg, nil
}

// isValidConfig returns true if at least one identifying field is populated.
func isValidConfig(cfg *Config) bool {
	return cfg.DefaultNamespace != "" || cfg.ClusterName != ""
}
