package cli

import (
	"github.com/metorial/cli/internal/app"
	authcmd "github.com/metorial/cli/internal/commands/auth"
	completioncmd "github.com/metorial/cli/internal/commands/completion"
	instancecmd "github.com/metorial/cli/internal/commands/instance"
	integrationscmd "github.com/metorial/cli/internal/commands/integrations"
	settingscmd "github.com/metorial/cli/internal/commands/settings"
	systemcmd "github.com/metorial/cli/internal/commands/system"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/spf13/cobra"
)

func newConsumerRoot(application *app.App) (*cobra.Command, error) {
	command, _, ctx, err := newBaseRoot(application, rootDefinition{
		kind:  rootKindConsumer,
		use:   "metorial",
		short: "CLI for Metorial integrations and MCP tools",
		long:  commandutil.ConsumerRootLongDescription(),
	})
	if err != nil {
		return nil, err
	}

	command.AddCommand(systemcmd.NewVersionCommand())
	command.AddCommand(systemcmd.NewFeedbackCommand())
	command.AddCommand(integrationscmd.NewCommand(ctx))

	if !commandutil.BrowserShellEnabled() {
		command.AddCommand(systemcmd.NewUpgradeCommand(application))
		command.AddCommand(systemcmd.NewOpenCommand())
		command.AddCommand(authcmd.NewCommand(ctx))
		command.AddCommand(authcmd.NewLoginCommand(ctx))
		command.AddCommand(authcmd.NewLogoutCommand())
		command.AddCommand(instancecmd.NewCommand(ctx))
		command.AddCommand(authcmd.NewProfileCommand(ctx))
		command.AddCommand(settingscmd.NewCommand(ctx))
		command.AddCommand(completioncmd.NewCommand(command.OutOrStdout()))
	}

	addMovedCommands(command)

	return command, nil
}
