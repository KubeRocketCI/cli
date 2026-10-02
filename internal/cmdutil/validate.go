package cmdutil

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// GuardFlagValues prepends validateStringFlags to the Args check of every
// runnable leaf command under root; a leaf without Args gets
// cobra.ArbitraryArgs. Group commands keep their nil Args and cobra's
// unknown-command check. Call once, after every AddCommand and after
// InitDefaultHelpCmd and InitDefaultCompletionCmd on the root; cobra otherwise
// adds those two commands inside Execute, unwrapped.
func GuardFlagValues(root *cobra.Command) {
	for _, c := range root.Commands() {
		GuardFlagValues(c)
	}

	if root.HasSubCommands() || !root.Runnable() {
		return
	}

	args := root.Args
	if args == nil {
		args = cobra.ArbitraryArgs
	}

	root.Args = func(cmd *cobra.Command, a []string) error {
		if err := validateStringFlags(cmd); err != nil {
			return err
		}

		return args(cmd, a)
	}
}

// validateStringFlags rejects a set string, stringSlice or stringArray flag
// with a value starting with "-": pflag reads "--status --pr 53" as
// status="--pr".
func validateStringFlags(cmd *cobra.Command) error {
	var errFlag string

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if errFlag != "" || !f.Changed {
			return
		}

		var values []string

		switch f.Value.Type() {
		case "string":
			values = []string{f.Value.String()}
		case "stringSlice", "stringArray":
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				values = sv.GetSlice()
			}
		}

		for _, v := range values {
			if strings.HasPrefix(v, "-") {
				errFlag = f.Name

				return
			}
		}
	})

	if errFlag != "" {
		return fmt.Errorf("flag needs an argument: --%s", errFlag)
	}

	return nil
}

// DNS-1123 label: lowercase alphanumerics and '-', 1..63 chars, start/end with
// alphanumeric. Boolean check for discovered config values; user-typed
// resource names go through ValidateK8sName.
var dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

const DNS1123SubdomainMaxLength = 253

func IsValidDNS1123Label(s string) bool {
	return dns1123LabelRegexp.MatchString(s)
}

// K8sNamePattern is the portal's tektonInputSchemas.k8sName shape: lowercase
// alphanumerics and '-', no dots, start/end alphanumeric. Length is bounded
// separately by DNS1123SubdomainMaxLength.
const K8sNamePattern = `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`

var k8sNameRegexp = regexp.MustCompile(K8sNamePattern)

// ValidateK8sName is the single validator for a Kubernetes resource name the
// user types: project, pipeline, deployment, env, cluster. placeholder names
// the argument in the error, e.g. "<project>" or "--cluster".
func ValidateK8sName(placeholder, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", placeholder)
	}

	if len(value) > DNS1123SubdomainMaxLength {
		return fmt.Errorf("%s must be at most %d characters (DNS-1123)", placeholder, DNS1123SubdomainMaxLength)
	}

	if !k8sNameRegexp.MatchString(value) {
		return fmt.Errorf("%s must be a valid DNS-1123 name: lowercase alphanumeric and '-', no dots", placeholder)
	}

	return nil
}
