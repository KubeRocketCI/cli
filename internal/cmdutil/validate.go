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

// DNS-1123 label: lowercase alphanumerics and '-', 1..63 chars, start/end with
// alphanumeric. Single source of truth across the CLI — do not duplicate.
var dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

const DNS1123SubdomainMaxLength = 253

func IsValidDNS1123Label(s string) bool {
	return dns1123LabelRegexp.MatchString(s)
}

// DNS-1123 subdomain: dot-separated label segments, up to 253 chars.
// Kubernetes resource names use the subdomain shape — not the 63-char label cap.
var dns1123SubdomainRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

func IsValidDNS1123Subdomain(s string) bool {
	if len(s) == 0 || len(s) > DNS1123SubdomainMaxLength {
		return false
	}

	return dns1123SubdomainRegexp.MatchString(s)
}
