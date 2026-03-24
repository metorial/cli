package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/metorial/cli/internal/terminal"
	"github.com/metorial/cli/internal/version"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	apiKey   string
	apiHost  string
	instance string
	profile  string
	format   string
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

const (
	commandCategoryGeneral  = "general"
	commandCategoryResource = "resource"
)

func Run() int {
	application := app.New()
	command, err := newRootCommand(application)
	if err != nil {
		_, _ = fmt.Fprintln(application.Stderr, err)
		return 1
	}

	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(application.Stderr, err)
		return 1
	}

	return 0
}

func newRootCommand(application *app.App) (*cobra.Command, error) {
	options := &rootOptions{}

	cobra.AddTemplateFunc("renderCommandSection", renderCommandSection)
	cobra.AddTemplateFunc("hasCommandCategory", hasCommandCategory)

	store, err := config.OpenStore()
	if err != nil {
		return nil, err
	}

	defaultFormat, err := resolveDefaultOutputFormat(store.Settings().DefaultFormat)
	if err != nil {
		return nil, err
	}

	command := &cobra.Command{
		Use:           "metorial",
		Short:         "CLI for the Metorial API and platform",
		Long:          rootLongDescription(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version.Version,
	}

	command.SetOut(application.Stdout)
	command.SetErr(application.Stderr)
	command.SetHelpCommand(newHelpCommand(command))
	command.SetHelpTemplate(helpTemplate())
	command.SetUsageTemplate(usageTemplate())

	command.PersistentFlags().StringVar(&options.apiKey, "api-key", "", "API key to use for authenticated requests")
	command.PersistentFlags().StringVar(&options.apiHost, "api-host", "", "API host or base URL (default: api.metorial.com)")
	command.PersistentFlags().StringVar(&options.instance, "instance", "", "Instance ID to use for organization-scoped tokens")
	command.PersistentFlags().StringVar(&options.profile, "profile", "", "Profile ID to use for authenticated requests")
	command.PersistentFlags().StringVar(&options.format, "format", defaultFormat, "Output format: yaml, toml, json, or structured")
	_ = command.RegisterFlagCompletionFunc("format", func(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"yaml", "toml", "json", "structured"}, cobra.ShellCompDirectiveNoFileComp
	})

	command.AddCommand(newVersionCommand())
	command.AddCommand(newFeedbackCommand())
	command.AddCommand(newOpenCommand())
	command.AddCommand(newLoginCommand(application, options))
	command.AddCommand(newLogoutCommand())
	command.AddCommand(newInstanceCommand(application, options))
	command.AddCommand(newProfileCommand(application, options))
	command.AddCommand(newSettingsCommand())
	command.AddCommand(newFetchCommand(application, options))
	command.AddCommand(newCompletionCommand(command.OutOrStdout()))

	if err := addPublicResourceCommands(command, application, options, resourcecmd.PublicResourcePlan()); err != nil {
		return nil, err
	}

	return command, nil
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

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(command *cobra.Command, args []string) {
			_, _ = fmt.Fprintf(
				command.OutOrStdout(),
				"metorial %s\ncommit: %s\nbuilt: %s\n",
				version.Version,
				version.Commit,
				version.Date,
			)
		},
	}
}

func newFeedbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "feedback",
		Short: "Share feedback or report issues",
		Run: func(command *cobra.Command, args []string) {
			link := terminal.Link("github.com/metorial/cli", config.DefaultFeedback)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Share feedback at %s\n", link)
		},
	}
}

func newOpenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the Metorial platform in your browser",
		RunE: func(command *cobra.Command, args []string) error {
			platformURL, err := config.ResolvePlatformURL()
			if err != nil {
				return err
			}

			if !browser.Supported() {
				_, _ = fmt.Fprintf(command.OutOrStdout(), "Open this URL in your browser: %s\n", platformURL)
				return nil
			}

			if err := browser.Open(platformURL); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(command.OutOrStdout(), "Opened %s\n", platformURL)
			return nil
		},
	}
}

func newLoginCommand(application *app.App, rootOptions *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in with OAuth and make the new profile current",
		Long: strings.TrimSpace(`
Start a browser-based OAuth sign-in flow and save the resulting profile to the
global CLI config in ~/.metorial/cli/config.json.

If a profile for the same organization and user already exists, it is updated in
place. The latest successful login becomes the current profile.
`),
		RunE: func(command *cobra.Command, args []string) error {
			return runLogin(command, application, rootOptions.apiHost)
		},
	}
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the current profile from this machine",
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			currentProfile, ok := store.CurrentProfile()
			if !ok {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "No active profile is configured.")
				_, _ = fmt.Fprintln(command.OutOrStdout(), "Run \"metorial login\" to add a profile.")
				return nil
			}

			removedProfile, err := store.RemoveProfile(currentProfile.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(command.OutOrStdout(), "Logged out from profile %s (%s).\n", removedProfile.Name, removedProfile.ID)

			nextProfile, ok := store.CurrentProfile()
			if ok {
				_, _ = fmt.Fprintf(
					command.OutOrStdout(),
					"Current profile is now %s (%s).\n",
					nextProfile.Name,
					nextProfile.ID,
				)
				return nil
			}

			_, _ = fmt.Fprintln(command.OutOrStdout(), "No profiles remain on this machine.")
			_, _ = fmt.Fprintln(command.OutOrStdout(), "Run \"metorial login\" to add a new profile.")
			return nil
		},
	}
}

func newProfileCommand(application *app.App, rootOptions *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:     "profile",
		Aliases: []string{"profiles"},
		Short:   "Manage saved OAuth profiles",
	}

	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			profiles := store.SortedProfiles()
			if len(profiles) == 0 {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "No saved profiles.")
				_, _ = fmt.Fprintln(command.OutOrStdout(), "Run \"metorial login\" or \"metorial profile add\" to create one.")
				return nil
			}

			currentProfile, _ := store.CurrentProfile()

			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(writer, "CURRENT\tID\tNAME\tORGANIZATION\tUSER\tAPI HOST\tEXPIRES")
			for _, profile := range profiles {
				marker := ""
				if currentProfile != nil && currentProfile.ID == profile.ID {
					marker = "*"
				}

				expires := "never"
				if !profile.ExpiresAt.IsZero() {
					expires = profile.ExpiresAt.Local().Format("2006-01-02 15:04")
				}

				userLabel := firstNonEmpty(profile.UserEmail, profile.UserName, profile.UserID)
				orgLabel := firstNonEmpty(profile.OrgName, profile.OrgID)
				apiHost := firstNonEmpty(profile.APIHost, store.Settings().DefaultAPIHost, config.DefaultAPIHost)

				_, _ = fmt.Fprintf(
					writer,
					"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					marker,
					profile.ID,
					profile.Name,
					orgLabel,
					userLabel,
					apiHost,
					expires,
				)
			}

			return writer.Flush()
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "set <profile-id>",
		Short: "Set the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			if _, ok := store.ProfileByID(args[0]); !ok {
				return profileNotFoundError(args[0])
			}

			if err := store.SetCurrentProfile(args[0]); err != nil {
				return err
			}

			profile, _ := store.ProfileByID(args[0])
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Current profile set to %s (%s).\n", profile.Name, profile.ID)
			return nil
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a profile using OAuth login",
		RunE: func(command *cobra.Command, args []string) error {
			return runLogin(command, application, rootOptions.apiHost)
		},
	})

	return command
}

func newFetchCommand(application *app.App, rootOptions *rootOptions) *cobra.Command {
	options := &fetch.Options{}

	command := &cobra.Command{
		Use:     "fetch <path-or-url>",
		Aliases: []string{"curl"},
		Short:   "Send an authenticated request to the Metorial API",
		Long: strings.TrimSpace(`
Send a raw HTTP request to the selected Metorial API host.

The request inherits authentication from --api-key, METORIAL_API_KEY,
METORIAL_TOKEN, or the current OAuth profile. When the current profile has a
refresh token, the CLI refreshes it automatically before requests when needed.
Use --profile to override the selected saved profile for this command. Use the
global --format flag to switch between YAML, TOML, JSON, and structured output.

Examples:
  metorial fetch /provider-listings
  metorial fetch /provider-listings -H 'X-Debug: true'
  metorial fetch /provider-listings -X POST -d '{"name":"demo"}'
  metorial curl https://api.metorial.com/provider-listings -i
`),
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			options.Target = args[0]

			response, requestErr := fetch.Execute(runtime, *options, application.Stdin)
			if response != nil {
				writer := command.OutOrStdout()
				if requestErr != nil {
					writer = command.ErrOrStderr()
				}
				if err := output.WriteResponse(writer, response, output.RenderOptions{
					Format:  format,
					Include: options.Include,
				}); err != nil {
					return err
				}
			}

			return requestErr
		},
	}

	command.Flags().StringSliceVarP(&options.Headers, "header", "H", nil, "Add request header in the form KEY: VALUE")
	command.Flags().StringVarP(&options.Method, "method", "X", "", "HTTP method to use")
	command.Flags().StringVarP(&options.Data, "data", "d", "", "Request body data, or @- to read from stdin")
	command.Flags().StringVar(&options.BodyFile, "body-file", "", "Read the request body from a file, or - for stdin")
	command.Flags().BoolVarP(&options.Include, "include", "i", false, "Include the response status line and headers in the output")

	return command
}

func newInstanceCommand(application *app.App, rootOptions *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:     "instance",
		Aliases: []string{"instances"},
		Short:   "List and inspect accessible instances",
	}

	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List instances available to the current token",
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveAuthConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile)
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

			return writeValue(command.OutOrStdout(), rootOptions.format, result)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "get <instance-id>",
		Short: "Get details for an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveAuthConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile)
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

			return writeValue(command.OutOrStdout(), rootOptions.format, result)
		},
	})

	return command
}

func newCompletionCommand(outputWriter io.Writer) *cobra.Command {
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

func newSettingsCommand() *cobra.Command {
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

			defaultHost := firstNonEmpty(store.Settings().DefaultAPIHost, "(not set)")
			defaultFormat := firstNonEmpty(store.Settings().DefaultFormat, string(output.FormatYAML))
			_, _ = fmt.Fprintf(command.OutOrStdout(), "default_api_host: %s\n", defaultHost)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "default_format: %s\n", defaultFormat)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "config_path: %s\n", store.Path())
			return nil
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

				_, _ = fmt.Fprintf(command.OutOrStdout(), "Default API host set to %s.\n", hostURL.String())
				return nil
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

				_, _ = fmt.Fprintf(command.OutOrStdout(), "Default format set to %s.\n", format)
				return nil
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

				_, _ = fmt.Fprintln(command.OutOrStdout(), "Default API host cleared.")
				return nil
			case "default-format":
				if err := store.UpdateSettings(func(settings *config.Settings) {
					settings.DefaultFormat = ""
				}); err != nil {
					return err
				}

				_, _ = fmt.Fprintln(command.OutOrStdout(), "Default format cleared.")
				return nil
			default:
				return fmt.Errorf("metorial: unknown setting %q. Supported settings: default-api-host, default-format", args[0])
			}
		},
	})

	return command
}

func resolveDefaultOutputFormat(raw string) (string, error) {
	format, err := output.ParseFormat(raw)
	if err != nil {
		return "", err
	}

	return string(format), nil
}

func normalizeSettingName(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func runLogin(command *cobra.Command, application *app.App, apiHostFlag string) error {
	store, err := config.OpenStore()
	if err != nil {
		return err
	}

	defaultHost := store.Settings().DefaultAPIHost
	if currentProfile, ok := store.CurrentProfile(); ok && strings.TrimSpace(currentProfile.APIHost) != "" {
		defaultHost = currentProfile.APIHost
	}

	apiHostURL, err := config.ResolveAPIHostWithDefault(apiHostFlag, defaultHost)
	if err != nil {
		return err
	}

	client := auth.NewClient(apiHostURL)
	startResponse, err := client.StartCLIAuth()
	if err != nil {
		return presentAuthError(err, "start login", "")
	}

	link := terminal.Link(startResponse.AuthorizationURL, startResponse.AuthorizationURL)

	_, _ = fmt.Fprintln(command.OutOrStdout(), "Starting browser sign-in.")
	if browser.Supported() {
		if err := browser.Open(startResponse.AuthorizationURL); err == nil {
			_, _ = fmt.Fprintln(command.OutOrStdout(), "Opened your browser automatically.")
		} else {
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Could not open your browser automatically: %v\n", err)
		}
	}

	_, _ = fmt.Fprintf(command.OutOrStdout(), "Open this URL if needed: %s\n", link)
	_, _ = fmt.Fprintf(command.OutOrStdout(), "Verification code: %s\n", startResponse.UserCode)
	_, _ = fmt.Fprintln(command.OutOrStdout(), "Waiting for sign-in to finish...")

	interval := time.Duration(maxInt(startResponse.Interval, 1)) * time.Second
	deadline := time.Now().Add(time.Duration(maxInt(startResponse.ExpiresIn, 1)) * time.Second)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("metorial: the login session expired before it was completed. Run \"metorial login\" to try again")
		}

		response, err := client.CompleteCLIAuth(startResponse.Token)
		if err == nil {
			profile, err := profileFromTokenResponse(store, apiHostURL.String(), response)
			if err != nil {
				return err
			}

			if err := store.UpsertProfile(*profile, true); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(command.OutOrStdout(), "Signed in as %s.\n", firstNonEmpty(profile.UserEmail, profile.UserName, profile.UserID))
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Current profile: %s (%s)\n", profile.Name, profile.ID)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Organization: %s\n", firstNonEmpty(profile.OrgName, profile.OrgID))
			return nil
		}

		apiError := &auth.Error{}
		if !errors.As(err, &apiError) {
			return err
		}

		switch apiError.ErrorCode {
		case "authorization_pending":
			time.Sleep(interval)
			continue
		case "slow_down":
			interval += 5 * time.Second
			time.Sleep(interval)
			continue
		case "access_denied":
			return fmt.Errorf("metorial: sign-in was cancelled. Run \"metorial login\" to try again")
		case "invalid_grant":
			return fmt.Errorf("metorial: the login session is no longer valid. Run \"metorial login\" to start a new one")
		default:
			return presentAuthError(err, "complete login", "Run \"metorial login\" to try again.")
		}
	}
}

func profileFromTokenResponse(store *config.Store, apiHost string, response *auth.TokenResponse) (*config.Profile, error) {
	if strings.TrimSpace(response.Organization.ID) == "" || strings.TrimSpace(response.User.ID) == "" {
		return nil, fmt.Errorf("metorial: login completed, but the server did not return enough profile information")
	}

	profileID := config.ProfileID(response.Organization.ID, response.User.ID)

	name := ""
	if existing, ok := store.ProfileByID(profileID); ok {
		name = existing.Name
	}
	if strings.TrimSpace(name) == "" {
		name = uniqueProfileName(store, profileID, response.Organization.Name)
	}

	now := time.Now().UTC()
	expiresAt := time.Time{}
	if response.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(response.ExpiresIn) * time.Second)
	}

	return &config.Profile{
		ID:           profileID,
		Name:         name,
		APIHost:      apiHost,
		ClientID:     response.ClientID,
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		TokenType:    firstNonEmpty(response.TokenType, "Bearer"),
		ExpiresAt:    expiresAt,
		OrgID:        response.Organization.ID,
		OrgName:      response.Organization.Name,
		UserID:       response.User.ID,
		UserName:     response.User.Name,
		UserEmail:    response.User.Email,
	}, nil
}

func uniqueProfileName(store *config.Store, profileID, organizationName string) string {
	base := slugify(organizationName)
	if base == "" {
		base = "profile"
	}

	if profileNameAvailable(store, profileID, base) {
		return base
	}

	for i := 0; i < 10; i++ {
		candidate := base + "-" + randomSuffix(4)
		if profileNameAvailable(store, profileID, candidate) {
			return candidate
		}
	}

	return base + "-" + randomSuffix(6)
}

func profileNameAvailable(store *config.Store, profileID, candidate string) bool {
	for _, profile := range store.SortedProfiles() {
		if profile.ID == profileID {
			continue
		}
		if profile.Name == candidate {
			return false
		}
	}
	return true
}

func profileNotFoundError(id string) error {
	return fmt.Errorf(
		"metorial: profile %q does not exist. Run \"metorial profiles list\" to see available profiles or \"metorial login\" to add a new one",
		id,
	)
}

func presentAuthError(err error, action string, hint string) error {
	apiError := &auth.Error{}
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode {
		case "cli_auth_disabled":
			return fmt.Errorf("metorial: CLI login is not enabled on this host")
		case "invalid_request":
			return withHint(fmt.Sprintf("metorial: could not %s because the authentication request was invalid", action), hint)
		case "invalid_grant":
			return withHint("metorial: the saved login session is no longer valid", hint)
		case "access_denied":
			return withHint("metorial: access was denied during login", hint)
		default:
			if strings.TrimSpace(apiError.ErrorMessage) != "" {
				return withHint(fmt.Sprintf("metorial: %s", apiError.ErrorMessage), hint)
			}
		}
	}

	return withHint(err.Error(), hint)
}

func withHint(message, hint string) error {
	if strings.TrimSpace(hint) == "" {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s\n%s", message, hint)
}

func writeValue(writer io.Writer, formatInput string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode response: %w", err)
	}

	format, err := output.ParseFormat(formatInput)
	if err != nil {
		return err
	}

	return output.WriteResponse(writer, &fetch.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       body,
	}, output.RenderOptions{
		Format: format,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func randomSuffix(length int) string {
	if length <= 0 {
		return ""
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "rand"
	}

	return hex.EncodeToString(bytes)[:length]
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func rootLongDescription() string {
	return strings.TrimSpace(`
Metorial gives you a fast way to work with the Metorial API and platform.

Use "metorial login" to sign in with OAuth, "metorial fetch" for raw
authenticated API requests, "metorial profile" to manage saved profiles, and
"metorial open" to launch the platform in a browser. Use --format yaml for the
default human-readable output, --format toml or --format json for serialized
output, or --format structured for tables and data lists.
`)
}

func helpTemplate() string {
	return `{{with (or .Long .Short)}}{{.}}

{{end}}Usage:
  {{.UseLine}}

{{if .HasAvailableSubCommands}}{{if hasCommandCategory .Commands "general"}}Commands:
{{renderCommandSection .Commands "general"}}{{end}}{{if hasCommandCategory .Commands "resource"}}
Resource Commands:
{{renderCommandSection .Commands "resource"}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}
`
}

func usageTemplate() string {
	return `Usage:
  {{.UseLine}}

{{if .HasAvailableSubCommands}}{{if hasCommandCategory .Commands "general"}}Commands:
{{renderCommandSection .Commands "general"}}{{end}}{{if hasCommandCategory .Commands "resource"}}
Resource Commands:
{{renderCommandSection .Commands "resource"}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`
}

func renderCommandSection(commands []*cobra.Command, category string) string {
	var buffer bytes.Buffer

	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) != category {
			continue
		}

		_, _ = fmt.Fprintf(&buffer, "  %-12s %s\n", command.Name(), command.Short)
	}

	return buffer.String()
}

func hasCommandCategory(commands []*cobra.Command, category string) bool {
	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) == category {
			return true
		}
	}

	return false
}

func commandCategory(command *cobra.Command) string {
	if command.Annotations != nil {
		if category := strings.TrimSpace(command.Annotations["metorial:command-category"]); category != "" {
			return category
		}
	}

	return commandCategoryGeneral
}
