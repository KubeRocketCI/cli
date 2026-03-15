// Package k8s provides Kubernetes API access for the krci CLI.
package k8s

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// ClientConfig holds connection parameters for the Kubernetes API server.
// All values are stored in krci config — no kubeconfig file dependency.
type ClientConfig struct {
	APIServer string
	CAData    string
	TokenFunc func(ctx context.Context) (string, error)
}

// NewDynamicClient creates a dynamic Kubernetes client using the provided config.
// The OIDC token is fetched via TokenFunc at creation time.
func NewDynamicClient(cfg ClientConfig) (dynamic.Interface, error) {
	if cfg.APIServer == "" {
		return nil, fmt.Errorf("API server URL is required")
	}

	u, err := url.Parse(cfg.APIServer)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("API server URL must use HTTPS with a valid host: %q", cfg.APIServer)
	}

	if cfg.TokenFunc == nil {
		return nil, fmt.Errorf("token function is required")
	}

	token, err := cfg.TokenFunc(context.Background())
	if err != nil {
		return nil, fmt.Errorf("obtaining access token: %w", err)
	}

	restCfg := &rest.Config{
		Host:        cfg.APIServer,
		BearerToken: token,
	}

	if cfg.CAData != "" {
		caBytes, err := base64.StdEncoding.DecodeString(cfg.CAData)
		if err != nil {
			return nil, fmt.Errorf("decoding CA data: %w", err)
		}

		restCfg.TLSClientConfig = rest.TLSClientConfig{
			CAData: caBytes,
		}
	}

	return dynamic.NewForConfig(restCfg)
}
