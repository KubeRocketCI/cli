package cmdutil

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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
// resource names go through ValidateK8sName or ValidateK8sSubdomain.
var dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

const DNS1123SubdomainMaxLength = 253

func IsValidDNS1123Label(s string) bool {
	return dns1123LabelRegexp.MatchString(s)
}

const k8sLabelPattern = `[a-z0-9]([a-z0-9-]*[a-z0-9])?`

// K8sNamePattern is the portal's tektonInputSchemas.k8sName shape: lowercase
// alphanumerics and '-', no dots, start/end alphanumeric. Length is bounded
// separately by DNS1123SubdomainMaxLength.
const K8sNamePattern = `^` + k8sLabelPattern + `$`

var k8sNameRegexp = regexp.MustCompile(K8sNamePattern)

// k8sSubdomainRegexp is the portal's k8sResourceNamePattern, the DNS-1123
// subdomain Kubernetes requires of any object name: k8sLabelPattern labels
// joined by dots.
var k8sSubdomainRegexp = regexp.MustCompile(`^` + k8sLabelPattern + `(\.` + k8sLabelPattern + `)*$`)

// ValidateK8sName validates a name the user types for an object the platform
// names: project, pipeline, deployment, env, cluster. A pod name goes through
// ValidateK8sSubdomain. placeholder names the argument in the error, e.g.
// "<project>" or "--cluster".
func ValidateK8sName(placeholder, value string) error {
	return validateDNS1123(placeholder, value, k8sNameRegexp, "lowercase alphanumeric and '-', no dots")
}

// ValidateK8sSubdomain validates the name of an object the platform does not
// name, such as a pod: Kubernetes accepts dots in it, ValidateK8sName does
// not.
func ValidateK8sSubdomain(placeholder, value string) error {
	return validateDNS1123(placeholder, value, k8sSubdomainRegexp, "lowercase alphanumeric, '-' and '.'")
}

// validateDNS1123 checks value against shape and the DNS-1123 length bound.
// allowed names the characters of the shape in the error.
func validateDNS1123(placeholder, value string, shape *regexp.Regexp, allowed string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", placeholder)
	}

	if len(value) > DNS1123SubdomainMaxLength {
		return fmt.Errorf("%s must be at most %d characters (DNS-1123)", placeholder, DNS1123SubdomainMaxLength)
	}

	if !shape.MatchString(value) {
		return fmt.Errorf("%s must be a valid DNS-1123 name: %s", placeholder, allowed)
	}

	return nil
}
