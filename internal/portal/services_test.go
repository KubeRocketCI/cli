package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func TestNewServices_Clock(t *testing.T) {
	t.Parallel()

	before := time.Now()
	assert.False(t, NewServices(nil, Target{}).now().Before(before), "default clock is not time.Now")

	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := NewServices(nil, Target{}, WithNow(func() time.Time { return fixed }))
	assert.Equal(t, fixed, s.now())
}

func TestServices_VerifySession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{name: "accepted", status: http.StatusOK},
		{name: "rejected", status: http.StatusUnauthorized, wantErr: ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"clusterName":"in-cluster","defaultNamespace":"platform","sonarWebUrl":"","dependencyTrackWebUrl":""}`))
			}))
			defer srv.Close()

			client, err := restapi.NewClientWithResponses(srv.URL + "/rest")
			require.NoError(t, err)

			err = NewServices(client, Target{}).VerifySession(t.Context())
			assert.Equal(t, "/rest/v1/config", gotPath)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestServices_VerifySession_TransportError(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial refused")

	client, err := restapi.NewClientWithResponses("http://portal.invalid/rest",
		restapi.WithHTTPClient(failingDoer{err: cause}))
	require.NoError(t, err)

	err = NewServices(client, Target{}).VerifySession(t.Context())
	require.ErrorIs(t, err, cause)
	assert.ErrorContains(t, err, "requesting cluster config: ")
}

// failingDoer fails every request with err.
type failingDoer struct{ err error }

func (d failingDoer) Do(*http.Request) (*http.Response, error) { return nil, d.err }

func TestServices_Accessors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		call     func(context.Context, *Services) error
		wantPath string
	}{
		{
			name: "sonar",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.Sonar().List(ctx, SonarListParams{Page: 1, PageSize: 50})
				return err
			},
			wantPath: "/rest/v1/sonar/list",
		},
		{
			name: "sca",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.SCA().List(ctx, SCAListParams{Page: 1, PageSize: 25})
				return err
			},
			wantPath: "/rest/v1/sca/list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(srv.Close)

			client, err := restapi.NewClientWithResponses(srv.URL + "/rest")
			require.NoError(t, err)

			require.ErrorIs(t, tt.call(t.Context(), NewServices(client, Target{})), ErrUnauthorized)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}
