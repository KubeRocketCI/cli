package cmdutil

import (
	"fmt"
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
