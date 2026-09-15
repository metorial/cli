package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type movedError struct {
	Command string
}

func (e *movedError) Error() string {
	return fmt.Sprintf("metorial: `%s` moved to `metorial-admin %s`", e.Command, e.Command)
}

func isMovedError(err error) bool {
	_, ok := asMovedError(err)
	return ok
}

func asMovedError(err error) (*movedError, bool) {
	if err == nil {
		return nil, false
	}

	moved, ok := err.(*movedError)
	return moved, ok
}

func addMovedCommands(root *cobra.Command) {
	names := []string{
		"providers",
		"deployments",
		"configs",
		"auth-configs",
		"identities",
		"actors",
		"identity-credentials",
		"sessions",
		"session-templates",
		"fetch",
		"curl",
		"example",
	}

	for _, name := range names {
		commandName := name
		command := &cobra.Command{
			Use:                commandName,
			Short:              fmt.Sprintf("Moved to metorial-admin %s", commandName),
			Args:               cobra.ArbitraryArgs,
			DisableFlagParsing: true,
			FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
			RunE: func(command *cobra.Command, args []string) error {
				return &movedError{Command: commandName}
			},
		}
		root.AddCommand(command)
	}
}
