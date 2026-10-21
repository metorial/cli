package onboarding

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/metorial/cli/internal/agent"
	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/browser"
	authcmd "github.com/metorial/cli/internal/commands/auth"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

func NewSignupCommand(ctx commandutil.Context) *cobra.Command {
	var region string

	command := &cobra.Command{
		Use:   "signup",
		Short: "Create a new Metorial organization without signing in",
		Long: strings.TrimSpace(`
Create a new, unclaimed Metorial organization named "<user>'s workspace" (using
this computer's user name) and save a profile for it on this machine. The
organization can be used right away. Open the printed claim link (or run "metorial-admin claim <email>") to attach it to a Metorial account.

The organization is created in a data region. You are asked to pick one, with the closest
region recommended. Pass --region to skip the question.
`),
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runSignup(command, ctx, region)
		},
	}

	command.Flags().StringVar(&region, "region", "", "Region to create the organization in (for example eu1 or us1)")

	return command
}

func NewClaimCommand(ctx commandutil.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "claim [email]",
		Short: "Claim the organization created with signup",
		Long: strings.TrimSpace(`
Print and open the claim link for the selected profile, which must be an
unclaimed organization created with "signup" on this machine. Use --profile to
claim a different one.

When an email is provided, Metorial sends the claim link to that address and
reminds it daily until the organization is claimed.
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			email := ""
			if len(args) > 0 {
				email = args[0]
			}
			return runClaim(command, ctx, email)
		},
	}
}

func runSignup(command *cobra.Command, ctx commandutil.Context, region string) error {
	store, err := config.OpenStore()
	if err != nil {
		return err
	}

	subjectType := ctx.App.SubjectType()
	defaultHost := store.Settings().DefaultAPIHost
	if currentProfile, ok := store.CurrentProfileFor(subjectType); ok && strings.TrimSpace(currentProfile.APIHost) != "" {
		defaultHost = currentProfile.APIHost
	}

	apiHostURL, err := config.ResolveAPIHostWithDefault(ctx.Options.APIHost, defaultHost)
	if err != nil {
		return err
	}

	stdoutFeatures := ctx.App.StdoutFeatures()
	isAgent, agentName := agent.IsAiAgent()

	region, err = resolveSignupRegion(command, ctx, apiHostURL, region, isAgent)
	if err != nil {
		return err
	}

	spinner := terminal.NewSpinner(command.OutOrStdout(), stdoutFeatures, "Creating your organization...")
	spinner.Start()
	response, err := auth.NewClient(apiHostURL).CLISignup(workspaceName(), region)
	spinner.Stop()
	if err != nil {
		return presentOnboardingError(err, command.Root().Name())
	}

	profile, err := authcmd.ProfileFromTokenResponse(store, subjectType, apiHostURL.String(), &response.TokenResponse)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	profile.Unclaimed = true
	profile.ClaimToken = response.Claim.Token
	profile.ClaimURL = response.Claim.URL
	profile.OrgSlug = response.Organization.Slug
	profile.InstanceID = response.Instance.ID
	profile.ClaimCheckedAt = &now

	if isAgent {
		if hint := detectEmailHint(agentName); hint != nil {
			profile.ClaimEmailHint = hint.Email
			profile.ClaimEmailHintSource = hint.Source
		}
	}

	if err := store.UpsertProfile(*profile, true); err != nil {
		return err
	}

	renderSignupSuccess(command.OutOrStdout(), stdoutFeatures, response, isAgent, *profile, command.Root().Name())

	if browser.Supported() {
		_ = browser.Open(response.Claim.URL)
	}

	return nil
}

func resolveSignupRegion(command *cobra.Command, ctx commandutil.Context, apiHost *url.URL, requested string, isAgent bool) (string, error) {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested, nil
	}

	features := ctx.App.StdoutFeatures()
	interactive := !isAgent && features.IsTTY && stdinIsTerminal(ctx.App)

	spinner := terminal.NewSpinner(command.OutOrStdout(), features, "Finding the closest region...")
	spinner.Start()
	choice := discoverRegions(apiHost)
	spinner.Stop()

	switch {
	case choice == nil:
		return "", nil
	case isAgent:
		return "", errors.New(choice.agentInstructions(command.Root().Name()))
	case interactive:
		return promptRegion(ctx.App, choice)
	default:
		return choice.Recommended, nil
	}
}

func runClaim(command *cobra.Command, ctx commandutil.Context, email string) error {
	store, err := config.OpenStore()
	if err != nil {
		return err
	}

	profile, err := findUnclaimedProfile(store, ctx.App.SubjectType(), ctx.Options.Profile, command.Root().Name())
	if err != nil {
		return err
	}

	runtime, err := ctx.App.ResolveAuthConfig(ctx.Options.APIKey, ctx.Options.APIHost, profile.ID)
	if err != nil {
		return err
	}

	client := auth.NewClient(runtime.APIHostURL)
	stdoutFeatures := ctx.App.StdoutFeatures()
	rootName := command.Root().Name()

	if strings.TrimSpace(email) != "" {
		if _, err := client.CLIClaimEmail(runtime.APIKey, profile.OrgID, profile.ClaimToken, email); err != nil {
			return presentOnboardingError(err, rootName)
		}

		saveClaimEmail(store, profile.ID, strings.TrimSpace(email))
		renderClaimEmailSent(command.OutOrStdout(), stdoutFeatures, *profile, strings.TrimSpace(email))
		if browser.Supported() {
			_ = browser.Open(profile.ClaimLink(time.Now()))
		}
		return nil
	}

	status, err := client.CLIOnboardingStatus(runtime.APIKey, profile.OrgID, profile.ClaimToken)
	if err != nil {
		return presentOnboardingError(err, rootName)
	}

	updated, claimed := applyStatus(store, profile.ID, status)
	if claimed {
		renderClaimedLine(command.OutOrStdout(), stdoutFeatures, updated)
		return nil
	}

	renderClaimLink(command.OutOrStdout(), stdoutFeatures, updated)
	if browser.Supported() {
		_ = browser.Open(updated.ClaimLink(time.Now()))
	}

	return nil
}

func findUnclaimedProfile(store *config.Store, subjectType, profileFlag, rootName string) (*config.Profile, error) {
	profile, ok := resolveSelectedProfile(store, subjectType, profileFlag)
	if !ok {
		return nil, fmt.Errorf("metorial: no profile is selected. Run \"%s signup\" to create an organization", rootName)
	}

	if !profile.Unclaimed || strings.TrimSpace(profile.ClaimToken) == "" {
		return nil, fmt.Errorf(
			"metorial: the selected profile (%s) is not an unclaimed organization.\nThe claim token only exists on the machine that ran \"signup\" and is removed once the organization is claimed. Use --profile <id> to select another profile",
			commandutil.FirstNonEmpty(profile.OrgSlug, profile.OrgID),
		)
	}

	return profile, nil
}

// applyStatus keeps the profile ID stable after a claim (the token now belongs to the claiming
// user, but the profile is still keyed by the original system user) — the same as token refresh.
func applyStatus(store *config.Store, profileID string, status *auth.OnboardingStatusResponse) (config.Profile, bool) {
	profile, ok := store.ProfileByID(profileID)
	if !ok {
		return config.Profile{}, false
	}

	updated := *profile.Clone()
	now := time.Now().UTC()
	updated.ClaimCheckedAt = &now

	if strings.TrimSpace(status.ClaimURL) != "" {
		updated.ClaimURL = status.ClaimURL
	}
	if strings.TrimSpace(status.Organization.Slug) != "" {
		updated.OrgSlug = status.Organization.Slug
	}

	claimed := status.Status == "claimed"
	if claimed {
		updated.Unclaimed = false
		updated.ClaimToken = ""
		updated.ClaimURL = ""
		if status.User != nil && strings.TrimSpace(status.User.ID) != "" {
			updated.UserID = status.User.ID
			updated.UserName = status.User.Name
			updated.UserEmail = status.User.Email
		}
	}

	_ = store.UpsertProfile(updated, false)

	return updated, claimed
}
