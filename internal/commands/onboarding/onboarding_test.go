package onboarding

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/claimlink"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

var agentEnvVars = []string{
	"ANTIGRAVITY_CLI_ALIAS",
	"CLAUDECODE",
	"CLINE_ACTIVE",
	"CODEX_SANDBOX",
	"CODEX_THREAD_ID",
	"CODEX_SANDBOX_NETWORK_DISABLED",
	"CODEX_CI",
	"CURSOR_AGENT",
	"GEMINI_CLI",
	"OPENCODE",
}

func setupStore(t *testing.T, profiles ...config.Profile) *config.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, envVar := range agentEnvVars {
		t.Setenv(envVar, "")
	}

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	for _, profile := range profiles {
		if err := store.UpsertProfile(profile, true); err != nil {
			t.Fatalf("UpsertProfile() error = %v", err)
		}
	}
	return store
}

func unclaimedProfile() config.Profile {
	return config.Profile{
		ID:          config.ProfileID("org_1", "usr_system"),
		Name:        "cli-org",
		APIHost:     "https://api.metorial.test",
		AccessToken: "token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
		OrgID:       "org_1",
		OrgSlug:     "brave-otter",
		UserID:      "usr_system",
		Unclaimed:   true,
		ClaimToken:  "mt_claim_abc",
		ClaimURL:    "https://platform.metorial.test/welcome/claim?token=eu1_c94352902e507e_1800000000_314636815b9894fe7fc230c4cd7d58ca",
	}
}

func newTestCommand(name string) (*cobra.Command, *app.App, *bytes.Buffer) {
	stderr := &bytes.Buffer{}
	application := app.NewWithIO(&bytes.Buffer{}, &bytes.Buffer{}, stderr, terminal.Features{}, terminal.Features{})
	application.SetSubjectType(config.SubjectTypeOrganizationMember)

	root := &cobra.Command{Use: "metorial-admin"}
	child := &cobra.Command{Use: name}
	root.AddCommand(child)
	return child, application, stderr
}

func stubStatus(t *testing.T, status *auth.OnboardingStatusResponse, err error) *int {
	t.Helper()
	calls := 0
	original := fetchOnboardingStatus
	fetchOnboardingStatus = func(*url.URL, string, string, string) (*auth.OnboardingStatusResponse, error) {
		calls++
		return status, err
	}
	t.Cleanup(func() { fetchOnboardingStatus = original })
	return &calls
}

func TestProfileJSONRoundTripKeepsClaimFields(t *testing.T) {
	profile := unclaimedProfile()
	checkedAt := time.Now().UTC().Truncate(time.Second)
	profile.ClaimCheckedAt = &checkedAt
	setupStore(t, profile)

	reopened, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	got, ok := reopened.ProfileByID(profile.ID)
	if !ok {
		t.Fatalf("profile %q not found", profile.ID)
	}
	if !got.Unclaimed || got.ClaimToken != profile.ClaimToken || got.ClaimURL != profile.ClaimURL || got.OrgSlug != profile.OrgSlug {
		t.Fatalf("claim fields not preserved: %+v", got)
	}
	if got.ClaimCheckedAt == nil || !got.ClaimCheckedAt.Equal(checkedAt) {
		t.Fatalf("ClaimCheckedAt = %v, want %v", got.ClaimCheckedAt, checkedAt)
	}
}

func TestNoticeSuppressedOutsideAgent(t *testing.T) {
	setupStore(t, unclaimedProfile())
	calls := stubStatus(t, &auth.OnboardingStatusResponse{Status: "unclaimed"}, nil)

	command, application, stderr := newTestCommand("servers")
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})

	if stderr.Len() != 0 || *calls != 0 {
		t.Fatalf("expected no notice outside an agent, got %q (calls=%d)", stderr.String(), *calls)
	}
}

func TestNoticeShownInAgent(t *testing.T) {
	setupStore(t, unclaimedProfile())
	t.Setenv("CLAUDECODE", "1")
	calls := stubStatus(t, &auth.OnboardingStatusResponse{Status: "unclaimed"}, nil)

	command, application, stderr := newTestCommand("servers")
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})

	output := stderr.String()
	if !strings.Contains(output, "UNCLAIMED Metorial organization (brave-otter)") {
		t.Fatalf("notice missing org slug: %q", output)
	}
	if !strings.Contains(output, "metorial-admin claim <email>") {
		t.Fatalf("notice missing claim command: %q", output)
	}
	if *calls != 1 {
		t.Fatalf("expected one status sync, got %d", *calls)
	}

	stderr.Reset()
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})
	if *calls != 1 {
		t.Fatalf("expected status sync to be throttled, got %d calls", *calls)
	}
	if stderr.Len() == 0 {
		t.Fatalf("expected notice to still print while throttled")
	}
}

func TestNoticeSkippedForOnboardingCommands(t *testing.T) {
	for _, name := range []string{"signup", "claim", "version", "completion", "logout", "upgrade"} {
		t.Run(name, func(t *testing.T) {
			setupStore(t, unclaimedProfile())
			t.Setenv("CURSOR_AGENT", "1")
			stubStatus(t, &auth.OnboardingStatusResponse{Status: "unclaimed"}, nil)

			command, application, stderr := newTestCommand(name)
			MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})
			if stderr.Len() != 0 {
				t.Fatalf("expected no notice for %s, got %q", name, stderr.String())
			}
		})
	}
}

func TestNoticeSkippedForClaimedProfile(t *testing.T) {
	profile := unclaimedProfile()
	profile.Unclaimed = false
	setupStore(t, profile)
	t.Setenv("CURSOR_AGENT", "1")
	calls := stubStatus(t, nil, errors.New("unexpected"))

	command, application, stderr := newTestCommand("servers")
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})
	if stderr.Len() != 0 || *calls != 0 {
		t.Fatalf("expected no notice for claimed profile, got %q", stderr.String())
	}
}

func TestNoticeUpdatesProfileWhenClaimed(t *testing.T) {
	profile := unclaimedProfile()
	setupStore(t, profile)
	t.Setenv("CURSOR_AGENT", "1")
	stubStatus(t, &auth.OnboardingStatusResponse{
		Status:       "claimed",
		Organization: auth.SignupOrgInfo{ID: "org_1", Slug: "brave-otter"},
		User:         &auth.UserInfo{ID: "usr_real", Name: "Ada", Email: "ada@example.com"},
	}, nil)

	command, application, stderr := newTestCommand("servers")
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})

	if !strings.Contains(stderr.String(), "claimed by ada@example.com") {
		t.Fatalf("expected claimed line, got %q", stderr.String())
	}

	store, _ := config.OpenStore()
	got, ok := store.ProfileByID(profile.ID)
	if !ok {
		t.Fatalf("profile disappeared after claim")
	}
	if got.Unclaimed || got.ClaimToken != "" || got.UserID != "usr_real" || got.UserEmail != "ada@example.com" {
		t.Fatalf("profile not updated after claim: %+v", got)
	}
}

func TestNoticeSurvivesStatusFailure(t *testing.T) {
	setupStore(t, unclaimedProfile())
	t.Setenv("CURSOR_AGENT", "1")
	calls := stubStatus(t, nil, errors.New("network down"))

	command, application, stderr := newTestCommand("servers")
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})
	MaybePrintUnclaimedNotice(application, command, &commandutil.RootOptions{})

	if !strings.Contains(stderr.String(), "UNCLAIMED") {
		t.Fatalf("expected notice despite status failure, got %q", stderr.String())
	}
	if *calls != 1 {
		t.Fatalf("expected failed status sync to be throttled, got %d calls", *calls)
	}
}

func TestFindUnclaimedProfileUsesSelectedProfile(t *testing.T) {
	claimed := unclaimedProfile()
	claimed.ID = config.ProfileID("org_2", "usr_2")
	claimed.OrgID = "org_2"
	claimed.OrgSlug = "claimed-org"
	claimed.Unclaimed = false
	claimed.ClaimToken = ""
	store := setupStore(t, claimed, unclaimedProfile())

	profile, err := findUnclaimedProfile(store, config.SubjectTypeOrganizationMember, "", "metorial-admin")
	if err != nil || profile.OrgID != "org_1" {
		t.Fatalf("current profile: profile = %+v, err = %v", profile, err)
	}

	profile, err = findUnclaimedProfile(store, config.SubjectTypeOrganizationMember, unclaimedProfile().ID, "metorial-admin")
	if err != nil || profile.OrgID != "org_1" {
		t.Fatalf("--profile: profile = %+v, err = %v", profile, err)
	}

	_, err = findUnclaimedProfile(store, config.SubjectTypeOrganizationMember, claimed.ID, "metorial-admin")
	if err == nil || !strings.Contains(err.Error(), "not an unclaimed organization") || !strings.Contains(err.Error(), "claimed-org") {
		t.Fatalf("claimed profile error = %v", err)
	}
}

func TestFindUnclaimedProfileWithoutSelection(t *testing.T) {
	store := setupStore(t)

	_, err := findUnclaimedProfile(store, config.SubjectTypeOrganizationMember, "", "metorial-admin")
	if err == nil || !strings.Contains(err.Error(), "no profile is selected") || !strings.Contains(err.Error(), "metorial-admin signup") {
		t.Fatalf("error = %v", err)
	}

	_, err = findUnclaimedProfile(store, config.SubjectTypeOrganizationMember, "missing", "metorial-admin")
	if err == nil || !strings.Contains(err.Error(), "no profile is selected") {
		t.Fatalf("unknown --profile error = %v", err)
	}
}

func TestPresentOnboardingError(t *testing.T) {
	tests := []struct {
		name string
		err  *auth.Error
		want string
	}{
		{name: "rate limited", err: &auth.Error{ErrorCode: "rate_limited", RetryAfter: 120}, want: "Try again in 2m0s"},
		{name: "blocked", err: &auth.Error{ErrorCode: "signup_blocked"}, want: "metorial-admin login"},
		{name: "disabled", err: &auth.Error{ErrorCode: "signup_disabled"}, want: "not enabled on this host"},
		{name: "invalid email", err: &auth.Error{ErrorCode: "invalid_claim_email", Reason: "disposable"}, want: "(disposable)"},
		{name: "conflict", err: &auth.Error{ErrorCode: "conflict"}, want: "already claimed"},
		{name: "fallback", err: &auth.Error{ErrorCode: "weird", ErrorMessage: "Something broke"}, want: "metorial: Something broke"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := presentOnboardingError(tt.err, "metorial-admin")
			if !strings.Contains(got.Error(), tt.want) {
				t.Fatalf("presentOnboardingError() = %q, want substring %q", got.Error(), tt.want)
			}
		})
	}
}

func TestWorkspaceName(t *testing.T) {
	tests := []struct {
		username string
		want     string
	}{
		{username: "tobias", want: "tobias's workspace"},
		{username: `CORP\ada`, want: "ada's workspace"},
		{username: "  ", want: ""},
	}

	for _, tt := range tests {
		original := currentUsername
		currentUsername = func() string { return tt.username }
		got := workspaceName()
		currentUsername = original

		if got != tt.want {
			t.Fatalf("workspaceName() with %q = %q, want %q", tt.username, got, tt.want)
		}
	}
}

func TestClaimLinksArePrintedOnTheirOwnLine(t *testing.T) {
	features := terminal.Features{Width: 40, HasUnicode: true}

	profile := unclaimedProfile()
	claimURL := profile.ClaimLink(time.Now())

	var link bytes.Buffer
	renderClaimLink(&link, features, profile)

	var signup bytes.Buffer
	renderSignupSuccess(&signup, features, &auth.SignupResponse{Claim: auth.ClaimInfo{URL: claimURL}}, false, profile, "metorial-admin")

	for name, rendered := range map[string]string{"claim": link.String(), "signup": signup.String()} {
		found := false
		for _, line := range strings.Split(rendered, "\n") {
			if strings.HasPrefix(line, "https://platform.metorial.test/welcome/claim?") && strings.Contains(line, claimlink.Identifier(profile.ClaimToken)) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected the unwrapped claim URL on its own line:\n%s", name, rendered)
		}
	}
}

func TestClaimLinkIsSignedFreshFromTheClaimToken(t *testing.T) {
	profile := unclaimedProfile()
	now := time.Unix(1800000000, 0)

	link, err := url.Parse(profile.ClaimLink(now))
	if err != nil {
		t.Fatal(err)
	}

	if link.Host != "platform.metorial.test" || link.Path != "/welcome/claim" {
		t.Fatalf("unexpected claim link base: %s", link)
	}
	if link.Query().Get("region") != "" {
		t.Fatalf("the region belongs in the token, not a query parameter: %s", link)
	}
	if got, want := link.Query().Get("token"), claimlink.Sign(profile.ClaimToken, claimlink.PurposeClaim, "eu1", now.Add(24*time.Hour)); got != want {
		t.Fatalf("token = %q, want %q", got, want)
	}
	if strings.Contains(link.String(), profile.ClaimToken) {
		t.Fatalf("claim link must not contain the claim token: %s", link)
	}
}

func TestClaimLinkFallsBackToTheStoredLink(t *testing.T) {
	profile := unclaimedProfile()
	profile.ClaimToken = ""

	if got := profile.ClaimLink(time.Now()); got != profile.ClaimURL {
		t.Fatalf("ClaimLink() = %q, want %q", got, profile.ClaimURL)
	}
}
