// Package portal provides a client for the KubeRocketCI Portal public API.
package portal

import (
	"encoding/json"
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
}

// tRPC v10 response envelope.
type tRPCResponse struct {
	Result struct {
		Data json.RawMessage `json:"data"`
	} `json:"result"`
}

// tRPC superjson wrapper (used by some Portal versions).
type superJSONWrapper struct {
	JSON Config `json:"json"`
}

// FetchConfig calls the Portal public config.get endpoint (no auth required)
// and returns the cluster configuration.
// The portal URL must use HTTPS.
func FetchConfig(portalURL string) (*Config, error) {
	u, err := url.Parse(portalURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("portal URL must use HTTPS with a valid host: %q", portalURL)
	}

	return fetchConfig(portalURL)
}

// fetchConfig performs the actual HTTP GET and response parsing.
func fetchConfig(portalURL string) (*Config, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(strings.TrimRight(portalURL, "/") + "/api/config.get")
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

// parseConfig extracts Config from tRPC response body.
// Handles both superjson-wrapped and plain data formats.
func parseConfig(body []byte) (*Config, error) {
	var trpc tRPCResponse
	if err := json.Unmarshal(body, &trpc); err != nil {
		return nil, fmt.Errorf("parsing portal config response: %w", err)
	}

	if len(trpc.Result.Data) == 0 {
		return nil, fmt.Errorf("empty data in portal config response")
	}

	// Try superjson format: {"result": {"data": {"json": {...}}}}
	var wrapped superJSONWrapper
	if err := json.Unmarshal(trpc.Result.Data, &wrapped); err == nil && wrapped.JSON.DefaultNamespace != "" {
		return &wrapped.JSON, nil
	}

	// Try plain format: {"result": {"data": {...}}}
	var cfg Config
	if err := json.Unmarshal(trpc.Result.Data, &cfg); err == nil && cfg.DefaultNamespace != "" {
		return &cfg, nil
	}

	return nil, fmt.Errorf("unexpected portal config response format")
}
