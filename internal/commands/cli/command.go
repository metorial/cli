package cli

import (
	authcmd "github.com/metorial/cli/internal/commands/auth"
	settingscmd "github.com/metorial/cli/internal/commands/settings"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/spf13/cobra"
)

func NewCommand(ctx commandutil.Context) *cobra.Command {
	command := &cobra.Command{
		Use:   "cli",
		Short: "Search commands and manage CLI profiles and settings",
	}

	command.AddCommand(newSearchCommand(ctx))

	if !commandutil.BrowserShellEnabled() {
		command.AddCommand(authcmd.NewProfileCommand(ctx))
		command.AddCommand(settingscmd.NewCommand(ctx))
	}

	return command
}
