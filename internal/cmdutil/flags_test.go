package cmdutil

import (
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
