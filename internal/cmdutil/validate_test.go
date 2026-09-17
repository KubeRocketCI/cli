package cmdutil

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateStringFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "valid string flags",
			args: []string{"--status", "succeeded", "--project", "my-app"},
		},
		{
			name: "no flags set",
			args: nil,
		},
		{
			name:    "long flag consumed as value",
			args:    []string{"--status", "--project", "my-app"},
			wantErr: "flag needs an argument: --status",
		},
		{
			name:    "short flag consumed as value",
			args:    []string{"--status", "-p", "my-app"},
			wantErr: "flag needs an argument: --status",
		},
		{
			name: "non-string flags ignored",
			args: []string{"--count", "5", "--verbose"},
		},
		{
			name: "unchanged string flag with default ignored",
			args: []string{"--count", "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}

			var status, project string
			var count int
			var verbose bool

			cmd.Flags().StringVar(&status, "status", "", "filter by status")
			cmd.Flags().StringVar(&project, "project", "", "filter by project")
			cmd.Flags().IntVar(&count, "count", 0, "number of items")
			cmd.Flags().BoolVar(&verbose, "verbose", false, "verbose output")

			require.NoError(t, cmd.ParseFlags(tt.args))

			err := ValidateStringFlags(cmd)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidDNS1123Label(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"single char", "a", true},
		{"simple label", "payments-api", true},
		{"digits allowed", "service-123", true},
		{"63 chars at limit", strings.Repeat("a", 63), true},

		{"empty rejected", "", false},
		{"uppercase rejected", "UPPER", false},
		{"underscore rejected", "has_underscore", false},
		{"leading dash rejected", "-leading", false},
		{"trailing dash rejected", "trailing-", false},
		{"64 chars over limit", strings.Repeat("a", 64), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidDNS1123Label(tc.in); got != tc.want {
				t.Errorf("IsValidDNS1123Label(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateK8sName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr string // empty = expect no error
	}{
		{"simple name", "my-app", ""},
		{"single char", "a", ""},
		{"digits allowed", "app-123", ""},
		{"63 chars ok", strings.Repeat("a", 63), ""},
		{"64 chars ok", strings.Repeat("a", 64), ""},
		{"253 chars ok", strings.Repeat("a", 253), ""},

		{"empty rejected", "", "must not be empty"},
		{"dot rejected", "my.app", "must be a valid DNS-1123 name: lowercase alphanumeric and '-', no dots"},
		{"uppercase rejected", "My-App", "must be a valid DNS-1123 name: lowercase alphanumeric and '-', no dots"},
		{"underscore rejected", "my_app", "must be a valid DNS-1123 name: lowercase alphanumeric and '-', no dots"},
		{"leading hyphen rejected", "-my-app", "must be a valid DNS-1123 name: lowercase alphanumeric and '-', no dots"},
		{"trailing hyphen rejected", "my-app-", "must be a valid DNS-1123 name: lowercase alphanumeric and '-', no dots"},
		{"254 chars rejected", strings.Repeat("a", 254), "must be at most 253 characters"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateK8sName("<project>", tc.in)
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

			if !strings.HasPrefix(err.Error(), "<project>") {
				t.Fatalf("expected error to carry the placeholder, got: %v", err)
			}
		})
	}
}
