package cli

import (
	"fmt"
	"strings"

	"github.com/metorial/cli/internal/app"
	authcmd "github.com/metorial/cli/internal/commands/auth"
	completioncmd "github.com/metorial/cli/internal/commands/completion"
	examplecmd "github.com/metorial/cli/internal/commands/example"
	fetchcmd "github.com/metorial/cli/internal/commands/fetch"
	instancecmd "github.com/metorial/cli/internal/commands/instance"
	integrationscmd "github.com/metorial/cli/internal/commands/integrations"
	resourcescmd "github.com/metorial/cli/internal/commands/resources"
	settingscmd "github.com/metorial/cli/internal/commands/settings"
	systemcmd "github.com/metorial/cli/internal/commands/system"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/metorial/cli/internal/update"
	"github.com/metorial/cli/internal/version"
	"github.com/spf13/cobra"
)

func Run() int {
	application := app.New()
	return RunArgs(application, nil)
}

func RunArgs(application *app.App, args []string) int {
	command, err := NewRootCommand(application)
	if err != nil {
		renderCLIError(application, err)
		return 1
	}

	if args != nil {
		command.SetArgs(args)
	}

	if err := command.Execute(); err != nil {
		renderCLIError(application, err)
		return 1
	}

	return 0
}

func NewRootCommand(application *app.App) (*cobra.Command, error) {
	options := &commandutil.RootOptions{}

	commandutil.RegisterTemplateFuncs()

	store, err := config.OpenStore()
	if err != nil {
		return nil, err
	}

	defaultFormat, err := resolveDefaultOutputFormat(store.Settings().DefaultFormat)
	if err != nil {
		return nil, err
	}

	options.Format = defaultFormat
	commandutil.ConfigureHelpFeatures(application.StdoutFeatures())
	ctx := commandutil.NewContext(application, options)

	command := &cobra.Command{
		Use:           "metorial",
		Short:         "CLI for the Metorial API and platform",
		Long:          commandutil.RootLongDescription(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version.Version,
	}

	command.SetOut(application.Stdout)
	command.SetErr(application.Stderr)
	commandutil.ConfigureCommand(command)
	command.SetHelpCommand(newHelpCommand(command))

	command.PersistentFlags().StringVar(&options.APIKey, "api-key", "", "API key to use for authenticated requests")
	command.PersistentFlags().StringVar(&options.APIHost, "api-host", "", "API host or base URL (default: api.metorial.com)")
	command.PersistentFlags().StringVar(&options.Instance, "instance", "", "Instance ID to use for organization-scoped tokens")
	command.PersistentFlags().StringVar(&options.Profile, "profile", "", "Profile ID to use for authenticated requests")
	command.PersistentFlags().StringVar(&options.Format, "format", defaultFormat, "Output format: yaml, toml, json, or structured")
	_ = command.RegisterFlagCompletionFunc("format", func(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"yaml", "toml", "json", "structured"}, cobra.ShellCompDirectiveNoFileComp
	})

	command.AddCommand(systemcmd.NewVersionCommand())
	command.AddCommand(systemcmd.NewFeedbackCommand())
	command.AddCommand(integrationscmd.NewCommand(ctx))
	command.AddCommand(fetchcmd.NewCommand(ctx))

	if !commandutil.BrowserShellEnabled() {
		command.AddCommand(systemcmd.NewUpgradeCommand(application))
		command.AddCommand(systemcmd.NewOpenCommand())
		command.AddCommand(authcmd.NewCommand(ctx))
		command.AddCommand(authcmd.NewLoginCommand(ctx))
		command.AddCommand(authcmd.NewLogoutCommand())
		command.AddCommand(instancecmd.NewCommand(ctx))
		command.AddCommand(authcmd.NewProfileCommand(ctx))
		command.AddCommand(examplecmd.NewCommand(ctx))
		command.AddCommand(settingscmd.NewCommand(ctx))
		command.AddCommand(completioncmd.NewCommand(command.OutOrStdout()))
	} else {
		_ = command.PersistentFlags().MarkHidden("api-key")
		_ = command.PersistentFlags().MarkHidden("api-host")
		_ = command.PersistentFlags().MarkHidden("instance")
		_ = command.PersistentFlags().MarkHidden("profile")
	}

	if err := resourcescmd.AddPublicCommands(command, ctx); err != nil {
		return nil, err
	}
	if err := resourcescmd.AddSessionCommands(command, ctx); err != nil {
		return nil, err
	}

	command.PersistentPreRunE = func(command *cobra.Command, args []string) error {
		if shouldSkipUpgradeNotice(command) {
			return nil
		}

		return update.MaybePrintUpgradeNotice(application.Stderr, application.StderrFeatures())
	}

	return command, nil
}

func newRootCommand(application *app.App) (*cobra.Command, error) {
	return NewRootCommand(application)
}

func newHelpCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			target, _, err := root.Find(args)
			if err != nil {
				return err
			}

			return target.Help()
		},
	}
}

func resolveDefaultOutputFormat(raw string) (string, error) {
	format, err := output.ParseFormat(raw)
	if err != nil {
		return "", err
	}

	return string(format), nil
}

func renderCLIError(application *app.App, err error) {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return
	}

	features := application.StderrFeatures()
	colors := terminal.NewColorizer(features)

	if strings.Contains(message, "metorial: no authentication found.") {
		_, _ = fmt.Fprintln(application.Stderr, colors.Warning("Authentication Required"))
		_, _ = fmt.Fprintln(application.Stderr)
		_, _ = fmt.Fprintln(application.Stderr, colors.Muted("Sign in with `metorial login` to use your saved profile on this machine."))
		_, _ = fmt.Fprintln(application.Stderr)
		_, _ = fmt.Fprintln(application.Stderr, colors.Notice("Other options"))
		_, _ = fmt.Fprintln(application.Stderr, colors.Muted("Use `--api-key` for a one-off request, or set `METORIAL_API_KEY` / `METORIAL_TOKEN`."))
		return
	}

	lines := strings.Split(message, "\n")
	_, _ = fmt.Fprintln(application.Stderr, colors.Warning(lines[0]))
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			_, _ = fmt.Fprintln(application.Stderr)
			continue
		}
		_, _ = fmt.Fprintln(application.Stderr, colors.Muted(trimmed))
	}
}

func shouldSkipUpgradeNotice(command *cobra.Command) bool {
	for current := command; current != nil; current = current.Parent() {
		switch current.Name() {
		case "upgrade", "completion":
			return true
		}
	}

	return false
}
