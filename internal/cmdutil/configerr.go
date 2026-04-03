package cmdutil

import (
	"errors"
	"fmt"
	"strings"
)

// ConfigOption describes one way to set a configuration value.
type ConfigOption struct {
	Flag      string // e.g. "--portal-url" (empty if not available)
	EnvVar    string // e.g. "KRCI_PORTAL_URL"
	ConfigKey string // e.g. "portal-url in ~/.config/krci/config.yaml"
}

// LoginHint suggests running "krci auth login" to configure the CLI.
const LoginHint = "Or run: krci auth login --portal-url <url>"

// PortalURLOption describes how to set the portal URL.
var PortalURLOption = ConfigOption{
	Flag:      "--portal-url",
	EnvVar:    "KRCI_PORTAL_URL",
	ConfigKey: "portal-url in ~/.config/krci/config.yaml",
}

// ClusterNameOption describes how to set the cluster name.
var ClusterNameOption = ConfigOption{
	EnvVar:    "KRCI_CLUSTER_NAME",
	ConfigKey: "cluster-name in ~/.config/krci/config.yaml",
}

// NamespaceOption describes how to set the namespace.
var NamespaceOption = ConfigOption{
	EnvVar:    "KRCI_NAMESPACE",
	ConfigKey: "namespace in ~/.config/krci/config.yaml",
}

// IssuerURLOption describes how to set the OIDC issuer URL.
var IssuerURLOption = ConfigOption{
	EnvVar:    "KRCI_ISSUER_URL",
	ConfigKey: "issuer-url in ~/.config/krci/config.yaml",
}

// ErrAuthRequired wraps a cause into a user-facing "authentication required" error
// with a hint to run "krci auth login".
func ErrAuthRequired(cause error) error {
	return fmt.Errorf("authentication required: %w\n\nRun: krci auth login", cause)
}

// ConfigNotSetError returns a formatted error for a missing configuration field.
// When prefix is empty, the error opens with "<fieldName> not configured".
// When prefix is non-empty, it is used verbatim as the opening line.
// hint, when non-empty, is appended after the options list.
func ConfigNotSetError(fieldName, prefix string, opts ConfigOption, hint string) error {
	var b strings.Builder

	if prefix != "" {
		b.WriteString(prefix)
	} else {
		fmt.Fprintf(&b, "%s not configured", fieldName)
	}

	b.WriteString("\n\nSet it via:\n")

	if opts.Flag != "" {
		fmt.Fprintf(&b, "  %s flag\n", opts.Flag)
	}

	fmt.Fprintf(&b, "  %s env var\n", opts.EnvVar)
	fmt.Fprintf(&b, "  %s", opts.ConfigKey)

	if hint != "" {
		fmt.Fprintf(&b, "\n\n%s", hint)
	}

	return errors.New(b.String())
}
