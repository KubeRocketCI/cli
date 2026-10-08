package cmdutil

import "github.com/spf13/cobra"

// FinalizeTree sets the argument contract of every command under root:
//   - Args run validateStringFlags first.
//   - nil Args means cobra.NoArgs; cobra's help command, which takes a command
//     path, gets cobra.ArbitraryArgs.
//   - A group without Run or RunE prints its help: a bare group exits 0, a
//     mistyped subcommand fails with unknown command.
//
// Root keeps cobra's own unknown-command check. Call once, after every
// AddCommand and after InitDefaultHelpCmd and InitDefaultCompletionCmd on the
// root; cobra otherwise adds those two commands inside Execute, unfinalized.
func FinalizeTree(root *cobra.Command) {
	for _, c := range root.Commands() {
		if c.Name() == "help" {
			finalize(c, cobra.ArbitraryArgs)
			continue
		}

		finalize(c, cobra.NoArgs)
	}
}

func finalize(c *cobra.Command, defaultArgs cobra.PositionalArgs) {
	for _, sub := range c.Commands() {
		finalize(sub, cobra.NoArgs)
	}

	if c.HasSubCommands() && c.Run == nil && c.RunE == nil {
		c.RunE = func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		}
	}

	if !c.Runnable() {
		return
	}

	args := c.Args
	if args == nil {
		args = defaultArgs
	}

	c.Args = func(cmd *cobra.Command, a []string) error {
		if err := validateStringFlags(cmd); err != nil {
			return err
		}

		return args(cmd, a)
	}
}
