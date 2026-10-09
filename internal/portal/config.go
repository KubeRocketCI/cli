package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// ClusterConfig holds configuration returned by the authenticated config endpoint.
type ClusterConfig struct {
	ClusterName      string `json:"clusterName"`
	DefaultNamespace string `json:"defaultNamespace"`
}

// FetchOIDCConfig calls the public /rest/v1/config/oidc endpoint (no auth)
// and returns the OIDC issuer URL for pre-login discovery.
// The portal URL must use HTTPS.
func FetchOIDCConfig(ctx context.Context, portalURL string) (string, error) {
	if err := validatePortalURL(portalURL); err != nil {
		return "", err
	}

	return fetchOIDCConfig(ctx, portalURL)
}

func fetchOIDCConfig(ctx context.Context, portalURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restURL(portalURL, "/v1/config/oidc"), nil)
	if err != nil {
		return "", fmt.Errorf("creating OIDC config request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting OIDC config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
// The portal URL must use HTTPS.
func FetchClusterConfig(ctx context.Context, portalURL, token string) (*ClusterConfig, error) {
	if err := validatePortalURL(portalURL); err != nil {
		return nil, err
	}

	return fetchClusterConfig(ctx, portalURL, token)
}

func fetchClusterConfig(ctx context.Context, portalURL, token string) (*ClusterConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restURL(portalURL, "/v1/config"), nil)
	if err != nil {
		return nil, fmt.Errorf("creating cluster config request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting cluster config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading cluster config response: %w", err)
	}

	if err := checkResponse(resp.StatusCode, body); err != nil {
		return nil, fmt.Errorf("cluster config: %w", err)
	}

	var cfg ClusterConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("parsing cluster config: %w", err)
	}

	return &cfg, nil
}

// VerifySession calls the authenticated /rest/v1/config endpoint through
// client, so the portal validates the client's bearer token. ErrUnauthorized
// means the portal rejected the token.
func VerifySession(ctx context.Context, client *restapi.ClientWithResponses) error {
	resp, err := client.ConfigGetWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("requesting cluster config: %w", err)
	}

	return checkResponse(resp.StatusCode(), resp.Body)
}

func validatePortalURL(portalURL string) error {
	u, err := url.Parse(portalURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w with a valid host: %q", ErrHTTPSRequired, portalURL)
	}

	return nil
}

func restURL(portalURL, path string) string {
	return portalURL + "/rest" + path
}
