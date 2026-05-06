package pipelineruninternal

import (
	"strings"
	"testing"
)

func TestValidatePipelineName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"valid", "foo-build", ""},
		{"valid digits", "build-1", ""},
		{"valid 64 chars", strings.Repeat("a", 64), ""},
		{"empty", "", "must not be empty"},
		{"uppercase", "Foo-Build", "valid DNS-1123 subdomain"},
		{"underscore", "foo_build", "valid DNS-1123 subdomain"},
		{"leading dash", "-foo", "valid DNS-1123 subdomain"},
		{"trailing dash", "foo-", "valid DNS-1123 subdomain"},
		{"too long", strings.Repeat("a", 254), "valid DNS-1123 subdomain"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePipelineName(tc.input)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}

			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error to contain %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateOutputAndDryRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		format  string
		dryRun  bool
		wantSub string // empty = expect no error
	}{
		// Live-create path.
		{"empty default", "", false, ""},
		{"table", "table", false, ""},
		{"json", "json", false, ""},
		{"yaml without dry-run rejected", "yaml", false, "-o yaml requires --dry-run"},

		// Dry-run path.
		{"dry-run + empty default ok", "", true, ""},
		{"dry-run + json ok", "json", true, ""},
		{"dry-run + yaml ok", "yaml", true, ""},
		{"dry-run + table rejected", "table", true, "--dry-run cannot use -o table"},

		// Unknown format always rejected.
		{"unknown", "xml", false, "unknown output format"},
		{"unknown + dry-run", "xml", true, "unknown output format"},
		{"uppercase rejected", "JSON", false, "unknown output format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateOutputAndDryRun(tc.format, tc.dryRun)

			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}

			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error to contain %q, got: %v", tc.wantSub, err)
			}
		})
	}
}

func TestParseKeyValueList_HappyPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []string
		kind string
		want map[string]string
	}{
		{"nil input", nil, KindParameter, nil},
		{"empty input", []string{}, KindParameter, nil},
		{"single", []string{"k=v"}, KindParameter, map[string]string{"k": "v"}},
		{"multiple order independent", []string{"a=1", "b=2"}, KindParameter, map[string]string{"a": "1", "b": "2"}},
		{"value contains equals", []string{"token=abc=def=="}, KindParameter, map[string]string{"token": "abc=def=="}},
		{"whitespace trimmed", []string{"  k  =  v  "}, KindParameter, map[string]string{"k": "v"}},
		{"comma value preserved", []string{"items=v1,v2"}, KindParameter, map[string]string{"items": "v1,v2"}},
		{"json value preserved", []string{`items=["v1","v2"]`}, KindParameter, map[string]string{"items": `["v1","v2"]`}},
		{"empty value allowed", []string{"k="}, KindParameter, map[string]string{"k": ""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseKeyValueList(tc.in, tc.kind)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("length mismatch: got %d, want %d", len(got), len(tc.want))
			}

			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseKeyValueList_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      []string
		kind    string
		wantSub string
	}{
		{"malformed", []string{"keywithoutvalue"}, KindParameter, "parameter must be key=value"},
		{"empty key", []string{"=value"}, KindParameter, "parameter key must not be empty"},
		{"whitespace empty key", []string{"  =value"}, KindParameter, "parameter key must not be empty"},
		{"duplicate keys", []string{"k=v1", "k=v2"}, KindParameter, `duplicate parameter 'k'`},
		{"label kind word", []string{"keywithoutvalue"}, KindLabel, "label must be key=value"},
		{"label dup", []string{"k=v", "k=v2"}, KindLabel, `duplicate label 'k'`},
		{"label empty key", []string{"=v"}, KindLabel, "label key must not be empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseKeyValueList(tc.in, tc.kind)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}

			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error to contain %q, got: %v", tc.wantSub, err)
			}
		})
	}
}

func TestParseKeyValueList_NoLastWins(t *testing.T) {
	t.Parallel()

	got, err := ParseKeyValueList([]string{"k=v1", "k=v2"}, KindParameter)
	if err == nil {
		t.Fatalf("expected duplicate-key error, got: %v", got)
	}

	if !strings.Contains(err.Error(), `duplicate parameter 'k'`) {
		t.Fatalf("expected duplicate-key error message, got: %v", err)
	}
}

func TestHeaders_Stable(t *testing.T) {
	t.Parallel()

	want := []string{"NAME", "STATUS", "PROJECT", "PR", "AUTHOR", "TYPE", "STARTED", "DURATION"}
	if len(Headers) != len(want) {
		t.Fatalf("headers length: got %d, want %d", len(Headers), len(want))
	}

	for i, h := range want {
		if Headers[i] != h {
			t.Fatalf("Headers[%d]: got %q, want %q", i, Headers[i], h)
		}
	}
}
