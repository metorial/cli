package instance

import (
	"github.com/metorial/cli/internal/commandutil"
	"github.com/spf13/cobra"
)

func NewCommand(ctx commandutil.Context) *cobra.Command {
	command := &cobra.Command{
		Use:     "instance",
		Aliases: []string{"instances"},
		Short:   "List and inspect accessible instances",
	}

	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List instances available to the current token",
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := ctx.ResolveAuthRuntime()
			if err != nil {
				return err
			}

			sdk, err := runtime.BareSDK()
			if err != nil {
				return err
			}

			result, err := sdk.Instances.List()
			if err != nil {
				return err
			}

			return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, result)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "get <instance-id>",
		Short: "Get details for an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := ctx.ResolveAuthRuntime()
			if err != nil {
				return err
			}

			sdk, err := runtime.BareSDK()
			if err != nil {
				return err
			}

			result, err := sdk.Instances.Get(args[0])
			if err != nil {
				return err
			}

			return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, result)
		},
	})

	return command
}
