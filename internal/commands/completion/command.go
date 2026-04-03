package completion

import (
	"io"

	"github.com/spf13/cobra"
)

func NewCommand(outputWriter io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}

	command.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completions",
		RunE: func(command *cobra.Command, args []string) error {
			return command.Root().GenBashCompletion(outputWriter)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completions",
		RunE: func(command *cobra.Command, args []string) error {
			return command.Root().GenZshCompletion(outputWriter)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completions",
		RunE: func(command *cobra.Command, args []string) error {
			return command.Root().GenFishCompletion(outputWriter, true)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "powershell",
		Short: "Generate PowerShell completions",
		RunE: func(command *cobra.Command, args []string) error {
			return command.Root().GenPowerShellCompletionWithDesc(outputWriter)
		},
	})

	return command
}
