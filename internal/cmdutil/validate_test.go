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
