package status

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

// mockTokenProvider implements auth.TokenProvider for testing.
type mockTokenProvider struct {
	tokenErr error
	info     *auth.UserInfo
	infoErr  error
}

func (m *mockTokenProvider) GetToken(_ context.Context) (string, error) { return "tok", m.tokenErr }
func (m *mockTokenProvider) Login(_ context.Context) error              { return nil }
func (m *mockTokenProvider) Logout() error                              { return nil }
func (m *mockTokenProvider) UserInfo() (*auth.UserInfo, error)          { return m.info, m.infoErr }

// runStatus executes `auth status` against tp with the given argv and returns
// what the command wrote to the factory's stdout.
func runStatus(t *testing.T, tp auth.TokenProvider, args ...string) (stdout string, err error) {
	t.Helper()

	f := cmdtest.NewFactory()
	f.TokenProvider = func() (auth.TokenProvider, error) { return tp, nil }

	cmd := NewCmdStatus(f, nil)
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.Execute()

	return f.IOStreams.Out.(*bytes.Buffer).String(), err
}

type envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Data          *struct {
		Authenticated bool     `json:"authenticated"`
		User          string   `json:"user"`
		Name          string   `json:"name"`
		Groups        []string `json:"groups"`
		ExpiresAt     *string  `json:"expiresAt"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func parseEnvelope(t *testing.T, stdout string) envelope {
	t.Helper()

	var env envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\nstdout=%s", err, stdout)
	}

	if env.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", env.SchemaVersion, SchemaVersion)
	}

	return env
}

func TestStatus_NotAuthenticated_ReturnsError(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{tokenErr: auth.ErrNotAuthenticated, infoErr: auth.ErrNotAuthenticated}

	stdout, err := runStatus(t, tp)
	if err == nil {
		t.Fatal("expected an error without a session")
	}

	if !errors.Is(err, auth.ErrNotAuthenticated) {
		t.Errorf("errors.Is(err, ErrNotAuthenticated) = false, err = %v", err)
	}

	if !strings.Contains(err.Error(), "krci auth login") {
		t.Errorf("error should tell the user to log in, got %q", err.Error())
	}

	if stdout != "" {
		t.Errorf("stdout should be empty in table mode, got %q", stdout)
	}
}

func TestStatus_NotAuthenticated_JSONErrorEnvelope(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{tokenErr: auth.ErrNotAuthenticated, infoErr: auth.ErrNotAuthenticated}

	stdout, err := runStatus(t, tp, "-o", "json")
	if err == nil {
		t.Fatal("expected an error without a session")
	}

	env := parseEnvelope(t, stdout)
	if env.Error == nil {
		t.Fatalf("expected an error envelope, got %s", stdout)
	}

	if !strings.Contains(env.Error.Message, "not authenticated") {
		t.Errorf("error.message = %q, want it to mention 'not authenticated'", env.Error.Message)
	}

	if env.Data != nil {
		t.Errorf("error envelope must not carry data, got %s", stdout)
	}
}

func TestStatus_ExpiredSession_ReturnsError(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{auth.ErrTokenExpired, auth.ErrRefreshFailed} {
		tp := &mockTokenProvider{tokenErr: cause, info: &auth.UserInfo{Email: "user@example.com"}}

		stdout, err := runStatus(t, tp)
		if err == nil {
			t.Fatalf("cause %v: expected an error for an expired session", cause)
		}

		if !errors.Is(err, cause) {
			t.Errorf("cause %v: errors.Is on the cause = false, err = %v", cause, err)
		}

		if !strings.Contains(err.Error(), "session expired") || !strings.Contains(err.Error(), "krci auth login") {
			t.Errorf("cause %v: error = %q, want 'session expired' and the login hint", cause, err.Error())
		}

		if stdout != "" {
			t.Errorf("cause %v: stdout should be empty in table mode, got %q", cause, stdout)
		}
	}
}

func TestStatus_ExpiredEnvToken_KeepsItsMessage(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{tokenErr: auth.ErrEnvTokenExpired}

	stdout, err := runStatus(t, tp, "-o", "json")
	if !errors.Is(err, auth.ErrEnvTokenExpired) {
		t.Fatalf("err = %v, want ErrEnvTokenExpired", err)
	}

	if strings.Contains(err.Error(), "krci auth login") {
		t.Errorf("error = %q: an expired KRCI_TOKEN is not fixed by a login", err.Error())
	}

	env := parseEnvelope(t, stdout)
	if env.Error == nil || env.Error.Message != auth.ErrEnvTokenExpired.Error() {
		t.Errorf("error envelope = %+v, want message %q", env.Error, auth.ErrEnvTokenExpired.Error())
	}
}

func TestStatus_Authenticated_JSON(t *testing.T) {
	t.Parallel()

	expires := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	tp := &mockTokenProvider{info: &auth.UserInfo{
		Email:     "user@example.com",
		Name:      "User Name",
		Groups:    []string{"admins", "developers"},
		ExpiresAt: expires,
	}}

	stdout, err := runStatus(t, tp, "-o", "json")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	env := parseEnvelope(t, stdout)
	if env.Data == nil {
		t.Fatalf("expected a data envelope, got %s", stdout)
	}

	if !env.Data.Authenticated {
		t.Error("data.authenticated = false, want true")
	}

	if env.Data.User != "user@example.com" || env.Data.Name != "User Name" {
		t.Errorf("data.user/name = %q/%q", env.Data.User, env.Data.Name)
	}

	if len(env.Data.Groups) != 2 {
		t.Errorf("data.groups = %v, want 2 entries", env.Data.Groups)
	}

	if env.Data.ExpiresAt == nil {
		t.Fatal("data.expiresAt = null, want RFC3339 timestamp")
	}

	got, parseErr := time.Parse(time.RFC3339, *env.Data.ExpiresAt)
	if parseErr != nil || !got.Equal(expires) {
		t.Errorf("data.expiresAt = %q, want %s", *env.Data.ExpiresAt, expires.Format(time.RFC3339))
	}
}

func TestStatus_Authenticated_Table(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{info: &auth.UserInfo{
		Email:     "user@example.com",
		Name:      "User Name",
		Groups:    []string{"admins"},
		ExpiresAt: time.Now().Add(time.Hour),
	}}

	stdout, err := runStatus(t, tp)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	for _, want := range []string{"User:", "user@example.com", "Status:", "Authenticated", "Groups:", "admins"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

// fakePortal answers the session check with status and records the calls.
type fakePortal struct {
	status        int
	calls         int
	path          string
	authorization string
	url           string
}

// start serves the fake portal over plain HTTP, the local-development setup,
// and returns a RestClient factory pointed at it.
func (p *fakePortal) start(t *testing.T) func() (*restapi.ClientWithResponses, error) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls++
		p.path = r.URL.Path
		p.authorization = r.Header.Get("Authorization")
		w.WriteHeader(p.status)
		_, _ = w.Write([]byte(`{"clusterName":"in-cluster","defaultNamespace":"ns","sonarWebUrl":"","dependencyTrackWebUrl":""}`))
	}))
	t.Cleanup(srv.Close)

	p.url = srv.URL

	return func() (*restapi.ClientWithResponses, error) {
		return restapi.NewClientWithResponses(srv.URL + "/rest")
	}
}

// TestStatus_EnvToken_FactoryClientSendsBearer runs the check through the
// production Factory, so the bearer token reaches the portal the way every
// portal command sends it.
func TestStatus_EnvToken_FactoryClientSendsBearer(t *testing.T) {
	t.Parallel()

	p := &fakePortal{status: http.StatusOK}
	p.start(t)

	tp := &mockTokenProvider{info: &auth.UserInfo{Email: "ci@example.com", FromEnv: true}}

	f := cmdutil.New()
	f.IOStreams = &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	f.Config = func() (*config.Config, error) {
		return &config.Config{PortalURL: p.url, ClusterName: "in-cluster", Namespace: "ns"}, nil
	}
	f.TokenProvider = func() (auth.TokenProvider, error) { return tp, nil }

	cmd := NewCmdStatus(f, nil)
	cmd.SetArgs([]string{"-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if p.calls != 1 || p.authorization != "Bearer tok" {
		t.Errorf("portal saw %d call(s) with Authorization %q, want 1 with %q", p.calls, p.authorization, "Bearer tok")
	}
}

// runStatusWithPortal executes `auth status` with restClient standing in for
// the factory's portal client.
func runStatusWithPortal(
	t *testing.T, tp auth.TokenProvider, restClient func() (*restapi.ClientWithResponses, error), args ...string,
) (stdout string, err error) {
	t.Helper()

	f := cmdtest.NewFactory()
	f.TokenProvider = func() (auth.TokenProvider, error) { return tp, nil }
	f.RestClient = restClient

	cmd := NewCmdStatus(f, nil)
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.Execute()

	return f.IOStreams.Out.(*bytes.Buffer).String(), err
}

func TestStatus_UnreadableClaims_PortalAccepts(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{infoErr: errors.New("invalid ID token format")}
	p := &fakePortal{status: http.StatusOK}

	stdout, err := runStatusWithPortal(t, tp, p.start(t), "-o", "json")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if p.calls != 1 || p.path != "/rest/v1/config" {
		t.Errorf("portal check = %d call(s) to %q, want 1 to /rest/v1/config", p.calls, p.path)
	}

	env := parseEnvelope(t, stdout)
	if env.Data == nil || !env.Data.Authenticated {
		t.Fatalf("expected data.authenticated=true, got %s", stdout)
	}

	if env.Data.Groups == nil {
		t.Errorf("data.groups should be [] not null: %s", stdout)
	}

	if env.Data.ExpiresAt != nil {
		t.Errorf("data.expiresAt should be null without user info: %s", stdout)
	}
}

func TestStatus_PortalRejects(t *testing.T) {
	t.Parallel()

	cases := map[string]*mockTokenProvider{
		"unreadable claims":          {infoErr: errors.New("invalid ID token format")},
		"readable KRCI_TOKEN claims": {info: &auth.UserInfo{Email: "forged@example.com", FromEnv: true}},
	}

	for name, tp := range cases {
		for _, format := range []string{"table", "json"} {
			p := &fakePortal{status: http.StatusUnauthorized}

			stdout, err := runStatusWithPortal(t, tp, p.start(t), "-o", format)
			if !errors.Is(err, portal.ErrUnauthorized) {
				t.Fatalf("%s/%s: err = %v, want it to wrap portal.ErrUnauthorized", name, format, err)
			}

			if !strings.Contains(err.Error(), "the portal rejected the token") {
				t.Errorf("%s/%s: error = %q, want 'the portal rejected the token'", name, format, err.Error())
			}

			if format == "table" {
				if stdout != "" {
					t.Errorf("%s/table: stdout should be empty, got %q", name, stdout)
				}

				continue
			}

			env := parseEnvelope(t, stdout)
			if env.Error == nil || env.Data != nil {
				t.Errorf("%s/json: want an error envelope without data, got %s", name, stdout)
			}
		}
	}
}

func TestStatus_EnvToken_PortalAccepts(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{info: &auth.UserInfo{Email: "ci@example.com", FromEnv: true}}
	p := &fakePortal{status: http.StatusOK}

	stdout, err := runStatusWithPortal(t, tp, p.start(t), "-o", "json")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if p.calls != 1 {
		t.Errorf("portal check ran %d time(s) for KRCI_TOKEN, want 1", p.calls)
	}

	env := parseEnvelope(t, stdout)
	if env.Data == nil || !env.Data.Authenticated || env.Data.User != "ci@example.com" {
		t.Errorf("want authenticated ci@example.com, got %s", stdout)
	}
}

func TestStatus_PortalUnreachable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	tp := &mockTokenProvider{infoErr: errors.New("invalid ID token format")}
	restClient := func() (*restapi.ClientWithResponses, error) {
		return restapi.NewClientWithResponses(url + "/rest")
	}

	_, err := runStatusWithPortal(t, tp, restClient)
	if err == nil {
		t.Fatal("an unverifiable token must not report authenticated")
	}

	if !strings.Contains(err.Error(), "verifying the token with the portal") {
		t.Errorf("error = %q, want the verification context", err.Error())
	}
}

func TestStatus_PortalNotConfigured(t *testing.T) {
	t.Parallel()

	notSet := errors.New("portal URL not configured")
	tp := &mockTokenProvider{infoErr: errors.New("invalid ID token format")}
	restClient := func() (*restapi.ClientWithResponses, error) { return nil, notSet }

	stdout, err := runStatusWithPortal(t, tp, restClient, "-o", "json")
	if !errors.Is(err, notSet) {
		t.Fatalf("err = %v, want the RestClient error", err)
	}

	if env := parseEnvelope(t, stdout); env.Error == nil {
		t.Errorf("want an error envelope, got %s", stdout)
	}
}

func TestStatus_StoredSession_SkipsPortal(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{info: &auth.UserInfo{Email: "user@example.com"}}
	p := &fakePortal{status: http.StatusUnauthorized}

	if _, err := runStatusWithPortal(t, tp, p.start(t), "-o", "json"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if p.calls != 0 {
		t.Errorf("portal check ran %d time(s) for a readable stored session", p.calls)
	}
}

func TestStatus_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	tp := &mockTokenProvider{info: &auth.UserInfo{Email: "user@example.com"}}

	_, err := runStatus(t, tp, "-o", "yaml")
	if err == nil {
		t.Fatal("expected error for -o yaml")
	}

	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("error = %v, want 'unknown output format'", err)
	}
}
