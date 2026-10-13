package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClusterConfig holds configuration returned by the authenticated config endpoint.
type ClusterConfig struct {
	ClusterName      string `json:"clusterName"`
	DefaultNamespace string `json:"defaultNamespace"`
}

const (
	// configTimeout caps each config endpoint request.
	configTimeout = 10 * time.Second
	// maxConfigBodyBytes caps the config endpoint response body.
	maxConfigBodyBytes = 1 << 20
)

// FetchOIDCConfig calls the public /rest/v1/config/oidc endpoint (no auth)
// and returns the OIDC issuer URL for pre-login discovery.
// portalURL must pass ValidatePortalURL.
func FetchOIDCConfig(ctx context.Context, portalURL string) (string, error) {
	if err := ValidatePortalURL(portalURL); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restURL(portalURL, "/v1/config/oidc"), nil)
	if err != nil {
		return "", fmt.Errorf("creating OIDC config request: %w", err)
	}

	resp, err := NewHTTPClient(configTimeout).Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting OIDC config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConfigBodyBytes))
	if err != nil {
		return "", fmt.Errorf("reading OIDC config response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC config returned HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	var result struct {
		OIDCIssuerURL string `json:"oidcIssuerUrl"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing OIDC config: %w", err)
	}

	if result.OIDCIssuerURL == "" {
		return "", fmt.Errorf("portal returned empty OIDC issuer URL")
	}

	return result.OIDCIssuerURL, nil
}

// FetchClusterConfig calls the authenticated /rest/v1/config endpoint
// and returns cluster name and default namespace.
// portalURL must pass ValidatePortalURL.
func FetchClusterConfig(ctx context.Context, portalURL, token string) (*ClusterConfig, error) {
	if err := ValidatePortalURL(portalURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restURL(portalURL, "/v1/config"), nil)
	if err != nil {
		return nil, fmt.Errorf("creating cluster config request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := NewHTTPClient(configTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting cluster config: %w", err)
	}

	return readClusterConfig(resp)
}

// readClusterConfig reads and closes the body of a /rest/v1/config response.
// A non-200 status returns the checkResponse error without added context. A
// 200 body must be a JSON object with the clusterName and defaultNamespace
// keys; their values may be empty.
func readClusterConfig(resp *http.Response) (*ClusterConfig, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConfigBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("reading cluster config response: %w", err)
	}

	if err := checkResponse(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var raw struct {
		ClusterName      *string `json:"clusterName"`
		DefaultNamespace *string `json:"defaultNamespace"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing cluster config: %w", err)
	}

	if raw.ClusterName == nil || raw.DefaultNamespace == nil {
		return nil, errors.New("parsing cluster config: response lacks clusterName or defaultNamespace")
	}

	return &ClusterConfig{ClusterName: *raw.ClusterName, DefaultNamespace: *raw.DefaultNamespace}, nil
}

func restURL(portalURL, path string) string {
	return portalURL + "/rest" + path
}
