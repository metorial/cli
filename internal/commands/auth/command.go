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

func NewCommand(ctx commandutil.Context) *cobra.Command {
	command := &cobra.Command{
		Use:    "auth",
		Short:  "Authentication commands",
		Hidden: true,
		Long: strings.TrimSpace(`
Manage Metorial CLI authentication flows and saved profiles.
`),
	}

	command.AddCommand(NewLoginCommand(ctx))
	command.AddCommand(NewLogoutCommand(ctx))
	command.AddCommand(NewProfileCommand(ctx))

	return command
}

func NewLogoutCommand(ctx commandutil.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the current profile from this machine",
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			subjectType := ctx.App.SubjectType()
			stdoutFeatures := terminal.DetectWriter(command.OutOrStdout())
			currentProfile, ok := store.CurrentProfileFor(subjectType)
			if !ok {
				colors := terminal.NewColorizer(stdoutFeatures)
				_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Bold("No active profile is configured."))
				_, _ = fmt.Fprintln(command.OutOrStdout())
				_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted("Run `metorial login` to add a profile."))
				return nil
			}

			removedProfile, err := store.RemoveProfile(currentProfile.ID)
			if err != nil {
				return err
			}

			nextProfile, ok := store.CurrentProfileFor(subjectType)
			if ok {
				renderLogoutSuccess(command.OutOrStdout(), stdoutFeatures, *removedProfile, nextProfile)
				return nil
			}

			renderLogoutSuccess(command.OutOrStdout(), stdoutFeatures, *removedProfile, nil)

			return nil
		},
	}
}

func NewProfileCommand(ctx commandutil.Context) *cobra.Command {
	command := &cobra.Command{
		Use:     "profile",
		Aliases: []string{"profiles"},
		Short:   "Manage saved auth profiles",
	}

	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		RunE: func(command *cobra.Command, args []string) error {
			store, err := config.OpenStore()
			if err != nil {
				return err
			}

			subjectType := ctx.App.SubjectType()
			profiles := store.SortedProfilesFor(subjectType)
			if len(profiles) == 0 {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "No saved profiles.")
				_, _ = fmt.Fprintf(command.OutOrStdout(), "Run \"%s login\" or \"%s add\" to create one.\n", command.Root().Name(), command.Parent().CommandPath())
				return nil
			}

			currentProfile, _ := store.CurrentProfileFor(subjectType)
			colors := terminal.NewColorizer(ctx.App.StdoutFeatures())
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Bold("Saved Profiles"))
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted("These are the profiles you are currently logged in with."))
			_, _ = fmt.Fprintln(command.OutOrStdout())

			scopeLabel, subjectLabel := profileLabels(subjectType)
			table := output.Table{
				Columns: []string{
					colors.Accent("Status"),
					colors.Accent("Profile"),
					colors.Accent(scopeLabel),
					colors.Accent(subjectLabel),
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

				userLabel := profileSubjectValue(profile)
				orgLabel := profileScopeValue(profile)
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
			_, _ = fmt.Fprintf(command.OutOrStdout(), "%s `%s set %s`\n", colors.Notice("Switch to a different profile with"), command.Parent().CommandPath(), profiles[0].Name)
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

			profile, err := findProfile(store, ctx.App.SubjectType(), args[0])
			if err != nil {
				return err
			}

			scopeLabel, subjectLabel := profileLabels(profile.Subject())
			_, _ = fmt.Fprintln(command.OutOrStdout(), "This is one of the profiles you are currently logged in with.")
			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Name: %s\n", profile.Name)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "%s: %s\n", scopeLabel, profileScopeValue(*profile))
			_, _ = fmt.Fprintf(command.OutOrStdout(), "%s: %s\n", subjectLabel, profileSubjectValue(*profile))
			_, _ = fmt.Fprintf(command.OutOrStdout(), "API host: %s\n", commandutil.FirstNonEmpty(profile.APIHost, store.Settings().DefaultAPIHost, config.DefaultAPIHost))
			if profile.ExpiresAt.IsZero() {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "Expires: never")
			} else {
				_, _ = fmt.Fprintf(command.OutOrStdout(), "Expires: %s\n", profile.ExpiresAt.Local().Format("2006-01-02 15:04"))
			}
			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Switch to this profile with `%s set %s`.\n", command.Parent().CommandPath(), profile.Name)
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

			profile, err := findProfile(store, ctx.App.SubjectType(), args[0])
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
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted(fmt.Sprintf("See what you can do next with `%s --help`.", command.Root().Name())))
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

	subjectType := ctx.App.SubjectType()
	defaultHost := store.Settings().DefaultAPIHost
	if currentProfile, ok := store.CurrentProfileFor(subjectType); ok && strings.TrimSpace(currentProfile.APIHost) != "" {
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
	startSubjectType := ""
	if subjectType == config.SubjectTypeConsumer {
		startSubjectType = subjectType
	}
	startResponse, err := client.StartCLIAuth(startSubjectType)
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
			profile, err := profileFromTokenResponse(store, subjectType, apiHostURL.String(), response)
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
			if subjectType == config.SubjectTypeConsumer {
				return fmt.Errorf("metorial: sign-in was cancelled. Run \"metorial login\" to try again\nThe Metorial CLI signs you in to a portal. If no portal was available to pick, ask your administrator to invite you to one.")
			}
			return fmt.Errorf("metorial: sign-in was cancelled. Run \"metorial login\" to try again")
		case "invalid_grant":
			return fmt.Errorf("metorial: the login session is no longer valid. Run \"metorial login\" to start a new one")
		default:
			return presentAuthError(err, "complete login", "Run \"metorial login\" to try again.")
		}
	}
}

func profileFromTokenResponse(store *config.Store, subjectType string, apiHost string, response *auth.TokenResponse) (*config.Profile, error) {
	subjectType = config.NormalizeSubjectType(subjectType)
	if config.NormalizeSubjectType(response.SubjectType) != subjectType {
		return nil, fmt.Errorf("metorial: login completed, but the server issued a %s token instead of a %s token", config.NormalizeSubjectType(response.SubjectType), subjectType)
	}

	profile := &config.Profile{
		APIHost:      apiHost,
		ClientID:     response.ClientID,
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		TokenType:    commandutil.FirstNonEmpty(response.TokenType, "Bearer"),
		OrgID:        response.Organization.ID,
		OrgName:      response.Organization.Name,
		UserID:       response.User.ID,
		UserName:     response.User.Name,
		UserEmail:    response.User.Email,
	}

	nameBase := response.Organization.Name
	if subjectType == config.SubjectTypeConsumer {
		if response.ConsumerProfile == nil || strings.TrimSpace(response.ConsumerProfile.ID) == "" || response.Instance == nil || strings.TrimSpace(response.Instance.ID) == "" {
			return nil, fmt.Errorf("metorial: login completed, but the server did not return enough profile information")
		}

		profile.SubjectType = config.SubjectTypeConsumer
		profile.ID = config.ProfileID(response.Instance.ID, response.ConsumerProfile.ID)
		profile.ConsumerProfileID = response.ConsumerProfile.ID
		profile.ConsumerName = response.ConsumerProfile.Name
		profile.ConsumerEmail = response.ConsumerProfile.Email
		profile.InstanceID = response.Instance.ID
		if response.Portal != nil {
			profile.PortalID = response.Portal.ID
			profile.PortalName = response.Portal.Name
			nameBase = response.Portal.Name
		}
	} else {
		if strings.TrimSpace(response.Organization.ID) == "" || strings.TrimSpace(response.User.ID) == "" {
			return nil, fmt.Errorf("metorial: login completed, but the server did not return enough profile information")
		}

		profile.ID = config.ProfileID(response.Organization.ID, response.User.ID)
	}

	if existing, ok := store.ProfileByID(profile.ID); ok {
		profile.Name = existing.Name
	}
	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = uniqueProfileName(store, profile.ID, nameBase)
	}

	if response.ExpiresIn > 0 {
		profile.ExpiresAt = time.Now().UTC().Add(time.Duration(response.ExpiresIn) * time.Second)
	}

	return profile, nil
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

func findProfile(store *config.Store, subjectType string, value string) (*config.Profile, error) {
	if profile, ok := store.ProfileByID(value); ok && profile.Subject() == config.NormalizeSubjectType(subjectType) {
		return profile, nil
	}

	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, profileNotFoundError(value)
	}

	var matched *config.Profile
	for _, profile := range store.SortedProfilesFor(subjectType) {
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
