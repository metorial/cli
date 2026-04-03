package auth

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
)

func TestRenderLoginAuthScreenIncludesBrowserMessageWhenSupported(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderLoginAuthScreen(&output, terminal.Features{}, "https://app.metorial.com/cli/auth", "ABCD-1234", true)

	rendered := output.String()
	if !strings.Contains(rendered, "Please sign in to your Metorial account.") {
		t.Fatalf("renderLoginAuthScreen() missing sign-in copy: %q", rendered)
	}
	if !strings.Contains(rendered, "Verification Code: ABCD-1234") {
		t.Fatalf("renderLoginAuthScreen() missing verification code: %q", rendered)
	}
	if !strings.Contains(rendered, "Opening browser for Metorial authentication.") {
		t.Fatalf("renderLoginAuthScreen() missing browser copy: %q", rendered)
	}
}

func TestRenderLoginSuccess(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderLoginSuccess(&output, terminal.Features{})

	rendered := output.String()
	if !strings.Contains(rendered, "Logged in to Metorial successfully.") {
		t.Fatalf("renderLoginSuccess() missing success message: %q", rendered)
	}
	if !strings.Contains(rendered, "Get started by running `metorial`.") {
		t.Fatalf("renderLoginSuccess() missing next step: %q", rendered)
	}
}

func TestRenderLogoutSuccessWithoutRemainingProfiles(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderLogoutSuccess(&output, terminal.Features{}, config.Profile{
		Name:      "testing",
		OrgName:   "Testing Org",
		UserEmail: "test@example.com",
	}, nil)

	rendered := output.String()
	if !strings.Contains(rendered, "Logged out from Metorial successfully.") {
		t.Fatalf("renderLogoutSuccess() missing success message: %q", rendered)
	}
	if !strings.Contains(rendered, "Removed Profile: testing") {
		t.Fatalf("renderLogoutSuccess() missing removed profile: %q", rendered)
	}
	if !strings.Contains(rendered, "No profiles remain on this machine.") {
		t.Fatalf("renderLogoutSuccess() missing empty state: %q", rendered)
	}
	if !strings.Contains(rendered, "Run `metorial login` to add a new profile.") {
		t.Fatalf("renderLogoutSuccess() missing next step: %q", rendered)
	}
}

func TestRenderLogoutSuccessWithNextProfile(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderLogoutSuccess(&output, terminal.Features{}, config.Profile{
		Name:      "testing",
		OrgName:   "Testing Org",
		UserEmail: "test@example.com",
	}, &config.Profile{
		Name: "backup",
	})

	rendered := output.String()
	if !strings.Contains(rendered, "Active profile: backup") {
		t.Fatalf("renderLogoutSuccess() missing next profile: %q", rendered)
	}
	if !strings.Contains(rendered, "Continue with the Metorial CLI using this profile.") {
		t.Fatalf("renderLogoutSuccess() missing continuation hint: %q", rendered)
	}
}
