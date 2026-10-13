package portal

import (
	"context"
	"fmt"
	"time"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// Target is the portal, cluster and namespace that services talk to.
type Target struct {
	PortalURL string
	Cluster   string
	Namespace string
}

// Option configures service constructors.
type Option func(*options)

type options struct {
	now func() time.Time
}

// WithNow sets the clock that measures the duration of runs still in progress.
// Default: time.Now. now must be non-nil.
func WithNow(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

func applyOptions(opts []Option) options {
	o := options{now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}

	return o
}

// Services builds the portal services of one Target over one client.
type Services struct {
	client *restapi.ClientWithResponses
	target Target
	now    func() time.Time
}

// NewServices returns the services of target over client. target is not
// validated.
func NewServices(client *restapi.ClientWithResponses, target Target, opts ...Option) *Services {
	o := applyOptions(opts)

	return &Services{client: client, target: target, now: o.now}
}

// VerifySession calls the authenticated /rest/v1/config endpoint, so the
// portal validates the client's bearer token. ErrUnauthorized means the portal
// rejected the token.
func (s *Services) VerifySession(ctx context.Context) error {
	resp, err := s.client.ConfigGetWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("requesting cluster config: %w", err)
	}

	return checkResponse(resp.StatusCode(), resp.Body)
}
