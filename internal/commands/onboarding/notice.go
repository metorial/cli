package onboarding

import (
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/metorial/cli/internal/agent"
	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/spf13/cobra"
)

const statusSyncInterval = 30 * time.Second

var fetchOnboardingStatus = func(apiHost *url.URL, accessToken, orgID, claimToken string) (*auth.OnboardingStatusResponse, error) {
	return auth.NewClient(apiHost).CLIOnboardingStatus(accessToken, orgID, claimToken)
}

func MaybePrintUnclaimedNotice(application *app.App, command *cobra.Command, options *commandutil.RootOptions) {
	if isAgent, _ := agent.IsAiAgent(); !isAgent || shouldSkipUnclaimedNotice(command) {
		return
	}

	store, err := config.OpenStore()
	if err != nil {
		return
	}

	profile, ok := resolveSelectedProfile(store, application.SubjectType(), options.Profile)
	if !ok || !profile.Unclaimed {
		return
	}

	rootName := command.Root().Name()
	writer := application.Stderr

	if shouldSyncStatus(*profile, time.Now().UTC()) {
		if status := syncStatus(store, *profile, options.APIHost); status != nil {
			updated, claimed := applyStatus(store, profile.ID, status)
			if claimed {
				renderClaimedLine(writer, application.StderrFeatures(), updated)
				return
			}
			profile = &updated
		} else {
			markStatusChecked(store, *profile)
		}
	}

	writeAgentInstructions(writer, rootName, *profile)
	_, _ = writer.Write([]byte("\n"))
}

func shouldSkipUnclaimedNotice(command *cobra.Command) bool {
	for current := command; current != nil; current = current.Parent() {
		switch current.Name() {
		case "signup", "claim", "version", "completion", "logout", "upgrade", "help":
			return true
		}
	}
	return false
}

func resolveSelectedProfile(store *config.Store, subjectType, profileFlag string) (*config.Profile, bool) {
	if id := strings.TrimSpace(profileFlag); id != "" {
		profile, ok := store.ProfileByID(id)
		if !ok || profile.Subject() != config.NormalizeSubjectType(subjectType) {
			return nil, false
		}
		return profile, true
	}
	return store.CurrentProfileFor(subjectType)
}

func shouldSyncStatus(profile config.Profile, now time.Time) bool {
	if profile.ClaimCheckedAt == nil {
		return true
	}
	return now.Sub(*profile.ClaimCheckedAt) > statusSyncInterval
}

func syncStatus(store *config.Store, profile config.Profile, apiHostFlag string) *auth.OnboardingStatusResponse {
	if strings.TrimSpace(profile.ClaimToken) == "" || profile.Expired(time.Now().UTC()) {
		return nil
	}

	defaultHost := commandutil.FirstNonEmpty(profile.APIHost, store.Settings().DefaultAPIHost)
	if strings.TrimSpace(os.Getenv(config.EnvAPIHost)) != "" {
		defaultHost = store.Settings().DefaultAPIHost
	}

	apiHostURL, err := config.ResolveAPIHostWithDefault(apiHostFlag, defaultHost)
	if err != nil {
		return nil
	}

	status, err := fetchOnboardingStatus(apiHostURL, profile.AccessToken, profile.OrgID, profile.ClaimToken)
	if err != nil {
		return nil
	}

	return status
}

func markStatusChecked(store *config.Store, profile config.Profile) {
	now := time.Now().UTC()
	profile.ClaimCheckedAt = &now
	_ = store.UpsertProfile(profile, false)
}

func saveClaimEmail(store *config.Store, profileID, email string) {
	profile, ok := store.ProfileByID(profileID)
	if !ok {
		return
	}

	updated := *profile.Clone()
	updated.ClaimEmail = email
	_ = store.UpsertProfile(updated, false)
}
