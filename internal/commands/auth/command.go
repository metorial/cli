package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

func NewLoginCommand(ctx commandutil.Context) *cobra.Command {
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
			return runLogin(command, ctx, ctx.Options.APIHost)
		},
	}
}

func NewLogoutCommand() *cobra.Command {
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
				_, _ = fmt.Fprintf(command.OutOrStdout(), "Current profile is now %s (%s).\n", nextProfile.Name, nextProfile.ID)
				return nil
			}

			_, _ = fmt.Fprintln(command.OutOrStdout(), "No profiles remain on this machine.")
			_, _ = fmt.Fprintln(command.OutOrStdout(), "Run \"metorial login\" to add a new profile.")
			return nil
		},
	}
}

func NewProfileCommand(ctx commandutil.Context) *cobra.Command {
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
			colors := terminal.NewColorizer(ctx.App.StdoutFeatures())
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Bold("Saved Profiles"))
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted("These are the profiles you are currently logged in with."))
			_, _ = fmt.Fprintln(command.OutOrStdout())

			table := output.Table{
				Columns: []string{
					colors.Accent("Status"),
					colors.Accent("Profile"),
					colors.Accent("Organization"),
					colors.Accent("User"),
					colors.Accent("API Host"),
					colors.Accent("Expires"),
				},
				Features: ctx.App.StdoutFeatures(),
				MaxWidth: ctx.App.StdoutFeatures().Width,
			}
			for _, profile := range profiles {
				status := colors.Muted("Saved")
				if currentProfile != nil && currentProfile.ID == profile.ID {
					status = colors.Success("Active")
				}

				expires := "never"
				if !profile.ExpiresAt.IsZero() {
					expires = profile.ExpiresAt.Local().Format("2006-01-02 15:04")
				}

				userLabel := commandutil.FirstNonEmpty(profile.UserEmail, profile.UserName, profile.UserID)
				orgLabel := commandutil.FirstNonEmpty(profile.OrgName, profile.OrgID)
				apiHost := commandutil.FirstNonEmpty(profile.APIHost, store.Settings().DefaultAPIHost, config.DefaultAPIHost)

				name := profile.Name
				if currentProfile != nil && currentProfile.ID == profile.ID {
					name = colors.Bold(profile.Name)
				}

				table.Rows = append(table.Rows, []string{status, name, orgLabel, userLabel, apiHost, expires})
			}

			if err := table.Render(command.OutOrStdout()); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "%s `metorial profile set %s`\n", colors.Notice("Switch to a different profile with"), profiles[0].Name)
			return nil
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "get <profile-name-or-id>",
		Short: "Show a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			profile, err := findProfile(store, args[0])
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(command.OutOrStdout(), "This is one of the profiles you are currently logged in with.")
			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Name: %s\n", profile.Name)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Organization: %s\n", commandutil.FirstNonEmpty(profile.OrgName, profile.OrgID))
			_, _ = fmt.Fprintf(command.OutOrStdout(), "User: %s\n", commandutil.FirstNonEmpty(profile.UserEmail, profile.UserName, profile.UserID))
			_, _ = fmt.Fprintf(command.OutOrStdout(), "API host: %s\n", commandutil.FirstNonEmpty(profile.APIHost, store.Settings().DefaultAPIHost, config.DefaultAPIHost))
			if profile.ExpiresAt.IsZero() {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "Expires: never")
			} else {
				_, _ = fmt.Fprintf(command.OutOrStdout(), "Expires: %s\n", profile.ExpiresAt.Local().Format("2006-01-02 15:04"))
			}
			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Switch to this profile with `metorial profile set %s`.\n", profile.Name)
			return nil
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "set <profile-name-or-id>",
		Short: "Set the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			profile, err := findProfile(store, args[0])
			if err != nil {
				return err
			}

			if err := store.SetCurrentProfile(profile.ID); err != nil {
				return err
			}

			colors := terminal.NewColorizer(ctx.App.StdoutFeatures())
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Success("Profile Updated"))
			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "%s %s\n", colors.Notice("Active profile:"), colors.Bold(profile.Name))
			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted("See what you can do next with `metorial --help`."))
			return nil
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a profile using OAuth login",
		RunE: func(command *cobra.Command, args []string) error {
			return runLogin(command, ctx, ctx.Options.APIHost)
		},
	})

	return command
}

func runLogin(command *cobra.Command, ctx commandutil.Context, apiHostFlag string) error {
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
	stdoutFeatures := ctx.App.StdoutFeatures()
	renderLoginWaitingScreenWithFeatures(command.OutOrStdout(), stdoutFeatures)
	spinner := terminal.NewSpinner(command.OutOrStdout(), stdoutFeatures, "Preparing authentication...")
	spinner.Start()
	startResponse, err := client.StartCLIAuth()
	spinner.Stop()
	if err != nil {
		return presentAuthError(err, "start login", "")
	}

	browserOpenSupported := browser.Supported()
	renderLoginAuthScreen(command.OutOrStdout(), stdoutFeatures, startResponse.AuthorizationURL, startResponse.UserCode, browserOpenSupported)
	if browserOpenSupported {
		_ = browser.Open(startResponse.AuthorizationURL)
	}

	interval := time.Duration(commandutil.MaxInt(startResponse.Interval, 1)) * time.Second
	deadline := time.Now().Add(time.Duration(commandutil.MaxInt(startResponse.ExpiresIn, 1)) * time.Second)

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

			renderLoginSuccess(command.OutOrStdout(), stdoutFeatures)
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
		TokenType:    commandutil.FirstNonEmpty(response.TokenType, "Bearer"),
		ExpiresAt:    expiresAt,
		OrgID:        response.Organization.ID,
		OrgName:      response.Organization.Name,
		UserID:       response.User.ID,
		UserName:     response.User.Name,
		UserEmail:    response.User.Email,
	}, nil
}

func uniqueProfileName(store *config.Store, profileID, organizationName string) string {
	base := commandutil.Slugify(organizationName)
	if base == "" {
		base = "profile"
	}

	if profileNameAvailable(store, profileID, base) {
		return base
	}

	for i := 0; i < 10; i++ {
		candidate := base + "-" + commandutil.RandomSuffix(4)
		if profileNameAvailable(store, profileID, candidate) {
			return candidate
		}
	}

	return base + "-" + commandutil.RandomSuffix(6)
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
	return fmt.Errorf("metorial: profile %q does not exist. Run \"metorial profiles list\" to see available profiles or \"metorial login\" to add a new one", id)
}

func findProfile(store *config.Store, value string) (*config.Profile, error) {
	if profile, ok := store.ProfileByID(value); ok {
		return profile, nil
	}

	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, profileNotFoundError(value)
	}

	var matched *config.Profile
	for _, profile := range store.SortedProfiles() {
		if profile.Name != normalized {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("metorial: multiple profiles are named %q. Use the profile ID instead.", normalized)
		}
		matched = profile.Clone()
	}

	if matched != nil {
		return matched, nil
	}

	return nil, profileNotFoundError(value)
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
