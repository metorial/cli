package consumer

import (
	"github.com/metorial/cli/internal/commandutil"
	"github.com/spf13/cobra"
)

func NewCommands(ctx commandutil.Context) []*cobra.Command {
	env := commandEnv{ctx: ctx}

	commands := []*cobra.Command{}
	commands = append(commands, newCatalogCommands(env)...)
	commands = append(commands, newSetupCommands(env)...)
	commands = append(commands, newIntegrationCommands(env)...)
	commands = append(commands, newToolCommands(env)...)
	commands = append(commands, newInstallCommands(env)...)
	commands = append(commands, newTokensCommand(env), newWhoamiCommand(env))

	for _, command := range commands {
		commandutil.SetCommandCategory(command, commandutil.CommandCategoryIntegrations)
	}

	return commands
}
