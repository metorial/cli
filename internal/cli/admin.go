package cli

import (
	"github.com/metorial/cli/internal/app"
	authcmd "github.com/metorial/cli/internal/commands/auth"
	clicmd "github.com/metorial/cli/internal/commands/cli"
	completioncmd "github.com/metorial/cli/internal/commands/completion"
	examplecmd "github.com/metorial/cli/internal/commands/example"
	fetchcmd "github.com/metorial/cli/internal/commands/fetch"
	filescmd "github.com/metorial/cli/internal/commands/files"
	instancecmd "github.com/metorial/cli/internal/commands/instance"
	resourcescmd "github.com/metorial/cli/internal/commands/resources"
	sandboxcmd "github.com/metorial/cli/internal/commands/sandbox"
	systemcmd "github.com/metorial/cli/internal/commands/system"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/spf13/cobra"
)

func newAdminRoot(application *app.App) (*cobra.Command, error) {
	application.SetSubjectType(config.SubjectTypeOrganizationMember)

	command, _, ctx, err := newBaseRoot(application, rootDefinition{
		kind:  rootKindAdmin,
		use:   "metorial-admin",
		short: "Admin CLI for the Metorial Magnetar API",
		long:  commandutil.AdminRootLongDescription(),
	})
	if err != nil {
		return nil, err
	}

	command.AddCommand(systemcmd.NewVersionCommand())
	command.AddCommand(systemcmd.NewFeedbackCommand())
	command.AddCommand(fetchcmd.NewCommand(ctx))
	command.AddCommand(clicmd.NewCommand(ctx))

	if !commandutil.BrowserShellEnabled() {
		command.AddCommand(systemcmd.NewUpgradeCommand(application))
		command.AddCommand(systemcmd.NewOpenCommand())
		command.AddCommand(authcmd.NewCommand(ctx))
		command.AddCommand(authcmd.NewLoginCommand(ctx))
		command.AddCommand(authcmd.NewLogoutCommand(ctx))
		command.AddCommand(instancecmd.NewCommand(ctx))
		command.AddCommand(examplecmd.NewCommand(ctx))
		command.AddCommand(completioncmd.NewCommand(command.OutOrStdout()))
	}

	if err := resourcescmd.AddGeneratedCommands(command, ctx); err != nil {
		return nil, err
	}
	if err := sandboxcmd.Attach(command, ctx); err != nil {
		return nil, err
	}
	if !commandutil.BrowserShellEnabled() {
		if err := filescmd.Attach(command, ctx); err != nil {
			return nil, err
		}
	}

	return command, nil
}
