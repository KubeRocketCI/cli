// Package portal provides a client for the KubeRocketCI Portal tRPC API.
package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tRPC procedure names for K8s resource access.
const (
	procedureK8sList = "k8s.list"
	procedureK8sGet  = "k8s.get"
)

// Client provides authenticated access to the KubeRocketCI Portal tRPC API.
type Client struct {
	baseURL   string
	tokenFunc func(ctx context.Context) (string, error)
	http      *http.Client
}

// NewClient creates a portal tRPC client.
// tokenFunc returns the idToken for Bearer authentication.
// The portalURL must use HTTPS to prevent Bearer tokens from being sent in cleartext.
func NewClient(portalURL string, tokenFunc func(context.Context) (string, error)) (*Client, error) {
	u, err := url.Parse(portalURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("%w: %q", ErrHTTPSRequired, portalURL)
	}

	return &Client{
		baseURL:   strings.TrimSuffix(portalURL, "/"),
		tokenFunc: tokenFunc,
		http:      &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Query calls a tRPC query procedure (HTTP GET) and unmarshals the result into out.
func (c *Client) Query(ctx context.Context, procedure string, input any, out any) error {
	reqURL := c.baseURL + "/api/" + procedure

	if input != nil {
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encoding tRPC input: %w", err)
		}

		reqURL += "?input=" + url.QueryEscape(string(inputJSON))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	token, err := c.tokenFunc(ctx)
	if err != nil {
		return fmt.Errorf("obtaining auth token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("portal request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("reading portal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return parseErrorResponse(body, resp.StatusCode)
	}

	return parseTRPCResult(body, out)
}

// tRPCErrorResponse represents a tRPC error envelope.
type tRPCErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
		Data    struct {
			Code       string `json:"code"`
			HTTPStatus int    `json:"httpStatus"`
		} `json:"data"`
	} `json:"error"`
}

// parseErrorResponse extracts a structured error from a tRPC error response.
func parseErrorResponse(body []byte, httpStatus int) error {
	var trpcErr tRPCErrorResponse
	if err := json.Unmarshal(body, &trpcErr); err == nil && trpcErr.Error.Data.Code != "" {
		return &APIError{
			Code:       trpcErr.Error.Data.Code,
			Message:    trpcErr.Error.Message,
			HTTPStatus: trpcErr.Error.Data.HTTPStatus,
		}
	}

	return fmt.Errorf("portal returned HTTP %d", httpStatus)
}

// parseTRPCResult extracts data from a tRPC response envelope into out.
// Handles both superjson {"result":{"data":{"json":...}}} and plain {"result":{"data":...}} formats.
func parseTRPCResult(body []byte, out any) error {
	var envelope struct {
		Result struct {
			Data json.RawMessage `json:"data"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("parsing tRPC response: %w", err)
	}

	if len(envelope.Result.Data) == 0 || string(envelope.Result.Data) == "null" {
		return fmt.Errorf("empty data in tRPC response")
	}

	// Try superjson format: superjson envelopes always contain both "json" and
	// "meta" keys. Checking for both prevents false positives when the actual
	// payload happens to have a root "json" field.
	var superJSON struct {
		JSON json.RawMessage `json:"json"`
		Meta json.RawMessage `json:"meta"`
	}

	sjErr := json.Unmarshal(envelope.Result.Data, &superJSON)
	if sjErr == nil && len(superJSON.JSON) > 0 && superJSON.Meta != nil {
		return json.Unmarshal(superJSON.JSON, out)
	}

	// Plain format: data IS the actual data.
	return json.Unmarshal(envelope.Result.Data, out)
}
