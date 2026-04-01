package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/terminal"
)

func TestRenderLoginAuthScreenIncludesBrowserMessageWhenSupported(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderLoginAuthScreen(&output, terminal.Features{}, "https://app.metorial.com/cli/auth", "ABCD-1234", true)

	rendered := output.String()
	if !strings.Contains(rendered, "Welcome to the Metorial CLI!") {
		t.Fatalf("renderLoginAuthScreen() missing welcome: %q", rendered)
	}
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

func TestRenderLoginWaitingScreenWithFeatures(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderLoginWaitingScreenWithFeatures(&output, terminal.Features{})

	if !strings.Contains(output.String(), "Welcome to the Metorial CLI!") {
		t.Fatalf("renderLoginWaitingScreenWithFeatures() missing welcome: %q", output.String())
	}
}
