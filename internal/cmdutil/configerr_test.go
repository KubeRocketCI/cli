package cmdutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigNotSetError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		prefix    string
		opts      ConfigOption
		hint      string
		want      string
	}{
		{
			name:      "all options with hint",
			fieldName: "portal URL",
			opts: ConfigOption{
				Flag:      "--portal-url",
				EnvVar:    "KRCI_PORTAL_URL",
				ConfigKey: "portal-url in ~/.config/krci/config.yaml",
			},
			hint: "Or run: krci auth login --portal-url <url>",
			want: "portal URL not configured\n\nSet it via:\n" +
				"  --portal-url flag\n" +
				"  KRCI_PORTAL_URL env var\n" +
				"  portal-url in ~/.config/krci/config.yaml\n\n" +
				"Or run: krci auth login --portal-url <url>",
		},
		{
			name:      "no flag",
			fieldName: "cluster name",
			opts: ConfigOption{
				EnvVar:    "KRCI_CLUSTER_NAME",
				ConfigKey: "cluster-name in ~/.config/krci/config.yaml",
			},
			hint: "Or run: krci auth login --portal-url <url>",
			want: "cluster name not configured\n\nSet it via:\n" +
				"  KRCI_CLUSTER_NAME env var\n" +
				"  cluster-name in ~/.config/krci/config.yaml\n\n" +
				"Or run: krci auth login --portal-url <url>",
		},
		{
			name:      "custom prefix",
			fieldName: "OIDC issuer URL",
			prefix:    "portal did not return an OIDC issuer URL (portal may be outdated)",
			opts: ConfigOption{
				EnvVar:    "KRCI_ISSUER_URL",
				ConfigKey: "issuer-url in ~/.config/krci/config.yaml",
			},
			want: "portal did not return an OIDC issuer URL (portal may be outdated)\n\nSet it via:\n" +
				"  KRCI_ISSUER_URL env var\n" +
				"  issuer-url in ~/.config/krci/config.yaml",
		},
		{
			name:      "no hint",
			fieldName: "portal URL",
			opts: ConfigOption{
				Flag:      "--portal-url",
				EnvVar:    "KRCI_PORTAL_URL",
				ConfigKey: "portal-url in ~/.config/krci/config.yaml",
			},
			want: "portal URL not configured\n\nSet it via:\n" +
				"  --portal-url flag\n" +
				"  KRCI_PORTAL_URL env var\n" +
				"  portal-url in ~/.config/krci/config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ConfigNotSetError(tt.fieldName, tt.prefix, tt.opts, tt.hint)
			assert.EqualError(t, err, tt.want)
		})
	}
}
