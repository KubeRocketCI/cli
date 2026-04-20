package cmdutil

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ValidateStringFlags checks that no string flag received a value that looks
// like another flag (e.g. "--status --pr 53" where "--pr" is consumed as the
// value for --status). Cobra/pflag silently accepts this; we reject it.
func ValidateStringFlags(cmd *cobra.Command) error {
	var errFlag string

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if errFlag != "" {
			return
		}

		if !f.Changed {
			return
		}

		if f.Value.Type() != "string" {
			return
		}

		v := f.Value.String()
		if strings.HasPrefix(v, "-") {
			errFlag = f.Name
		}
	})

	if errFlag != "" {
		return fmt.Errorf("flag needs an argument: --%s", errFlag)
	}

	return nil
}

// DNS-1123 label: lowercase alphanumerics and '-', must start and end with an
// alphanumeric, 1..63 chars. Used for Kubernetes namespaces, KubeRocketCI
// codebase names, and SonarQube projectKeys (which by Portal convention equal
// the codebase name). Single source of truth for the whole CLI.
var dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// DNS1123SubdomainMaxLength is the maximum length of a DNS-1123 subdomain.
const DNS1123SubdomainMaxLength = 253

// IsValidDNS1123Label reports whether s matches the DNS-1123 label shape.
func IsValidDNS1123Label(s string) bool {
	return dns1123LabelRegexp.MatchString(s)
}
