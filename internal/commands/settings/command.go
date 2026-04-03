package settings

import (
	"fmt"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCommand(ctx commandutil.Context) *cobra.Command {
	command := &cobra.Command{
		Use:   "settings",
		Short: "Manage global CLI settings",
	}

	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show current CLI settings",
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, map[string]any{
				"object":           "settings",
				"default_api_host": commandutil.FirstNonEmpty(store.Settings().DefaultAPIHost, "(not set)"),
				"default_format":   commandutil.FirstNonEmpty(store.Settings().DefaultFormat, string(output.FormatStructured)),
			})
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "set <setting> <value>",
		Short: "Set a global CLI setting",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			setting := normalizeSettingName(args[0])

			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			switch setting {
			case "default-api-host":
				hostURL, err := config.NormalizeBaseURL(args[1])
				if err != nil {
					return err
				}

				if err := store.UpdateSettings(func(settings *config.Settings) {
					settings.DefaultAPIHost = hostURL.String()
				}); err != nil {
					return err
				}

				return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, map[string]any{
					"object":      "settings.update",
					"action":      "set",
					"setting":     "default_api_host",
					"value":       hostURL.String(),
					"config_path": store.Path(),
				})
			case "default-format":
				format, err := resolveDefaultOutputFormat(args[1])
				if err != nil {
					return err
				}

				if err := store.UpdateSettings(func(settings *config.Settings) {
					settings.DefaultFormat = format
				}); err != nil {
					return err
				}

				return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, map[string]any{
					"object":      "settings.update",
					"action":      "set",
					"setting":     "default_format",
					"value":       format,
					"config_path": store.Path(),
				})
			default:
				return fmt.Errorf("metorial: unknown setting %q. Supported settings: default-api-host, default-format", args[0])
			}
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "unset <setting>",
		Short: "Clear a global CLI setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			setting := normalizeSettingName(args[0])

			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			switch setting {
			case "default-api-host":
				if err := store.UpdateSettings(func(settings *config.Settings) {
					settings.DefaultAPIHost = ""
				}); err != nil {
					return err
				}

				return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, map[string]any{
					"object":      "settings.update",
					"action":      "unset",
					"setting":     "default_api_host",
					"value":       "(not set)",
					"config_path": store.Path(),
				})
			case "default-format":
				if err := store.UpdateSettings(func(settings *config.Settings) {
					settings.DefaultFormat = ""
				}); err != nil {
					return err
				}

				return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, map[string]any{
					"object":      "settings.update",
					"action":      "unset",
					"setting":     "default_format",
					"value":       string(output.FormatStructured),
					"config_path": store.Path(),
				})
			default:
				return fmt.Errorf("metorial: unknown setting %q. Supported settings: default-api-host, default-format", args[0])
			}
		},
	})

	return command
}

func normalizeSettingName(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func resolveDefaultOutputFormat(raw string) (string, error) {
	format, err := output.ParseFormat(raw)
	if err != nil {
		return "", err
	}

	return string(format), nil
}
