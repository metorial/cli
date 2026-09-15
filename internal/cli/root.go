package cli

import (
	"fmt"
	"strings"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/metorial/cli/internal/update"
	"github.com/metorial/cli/internal/version"
	"github.com/spf13/cobra"
)

type rootKind string

const (
	rootKindConsumer rootKind = "consumer"
	rootKindAdmin    rootKind = "admin"
)

type rootDefinition struct {
	kind  rootKind
	use   string
	short string
	long  string
}

func Run() int {
	application := app.New()
	return RunArgs(application, nil)
}

func RunArgs(application *app.App, args []string) int {
	return runRoot(application, args, newConsumerRoot)
}

func RunAdmin() int {
	application := app.New()
	return RunAdminArgs(application, nil)
}

func RunAdminArgs(application *app.App, args []string) int {
	return runRoot(application, args, newAdminRoot)
}

func NewRootCommand(application *app.App) (*cobra.Command, error) {
	return newConsumerRoot(application)
}

func NewAdminRootCommand(application *app.App) (*cobra.Command, error) {
	return newAdminRoot(application)
}

func newRootCommand(application *app.App) (*cobra.Command, error) {
	return NewRootCommand(application)
}

func runRoot(
	application *app.App,
	args []string,
	build func(*app.App) (*cobra.Command, error),
) int {
	command, err := build(application)
	if err != nil {
		renderCLIError(application, err)
		return 1
	}

	if args != nil {
		command.SetArgs(args)
	}

	if err := command.Execute(); err != nil {
		renderCLIError(application, err)
		if isMovedError(err) {
			return 2
		}
		return 1
	}

	return 0
}

func newBaseRoot(application *app.App, definition rootDefinition) (*cobra.Command, *commandutil.RootOptions, commandutil.Context, error) {
	options := &commandutil.RootOptions{}

	commandutil.RegisterTemplateFuncs()

	store, err := config.OpenStore()
	if err != nil {
		return nil, nil, commandutil.Context{}, err
	}

	defaultFormat, err := resolveDefaultOutputFormat(store.Settings().DefaultFormat)
	if err != nil {
		return nil, nil, commandutil.Context{}, err
	}

	options.Format = defaultFormat
	commandutil.ConfigureHelpFeatures(application.StdoutFeatures())
	ctx := commandutil.NewContext(application, options)

	command := &cobra.Command{
		Use:           definition.use,
		Short:         definition.short,
		Long:          definition.long,
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

	if commandutil.BrowserShellEnabled() {
		_ = command.PersistentFlags().MarkHidden("api-key")
		_ = command.PersistentFlags().MarkHidden("api-host")
		_ = command.PersistentFlags().MarkHidden("instance")
		_ = command.PersistentFlags().MarkHidden("profile")
	}

	command.PersistentPreRunE = func(command *cobra.Command, args []string) error {
		if shouldSkipUpgradeNotice(command) {
			return nil
		}

		return update.MaybePrintUpgradeNotice(application.Stderr, application.StderrFeatures())
	}

	return command, options, ctx, nil
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
	if moved, ok := asMovedError(err); ok {
		features := application.StderrFeatures()
		colors := terminal.NewColorizer(features)
		_, _ = fmt.Fprintln(application.Stderr, colors.Warning("Command Moved"))
		_, _ = fmt.Fprintln(application.Stderr)
		_, _ = fmt.Fprintln(application.Stderr, colors.Muted(moved.Error()))
		_, _ = fmt.Fprintln(application.Stderr)
		_, _ = fmt.Fprintln(application.Stderr, colors.Notice("Next step"))
		_, _ = fmt.Fprintln(application.Stderr, colors.Muted(fmt.Sprintf("Install and run `metorial-admin %s` instead.", moved.Command)))
		return
	}

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
