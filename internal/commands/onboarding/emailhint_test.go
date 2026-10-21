package onboarding

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/claimlink"
	"github.com/metorial/cli/internal/config"
)

func stubHintCommands(t *testing.T, outputs map[string]string) *[]string {
	t.Helper()
	var calls []string
	original := runHintCommand
	runHintCommand = func(name string, args ...string) (string, error) {
		key := name + " " + strings.Join(args, " ")
		calls = append(calls, key)
		if output, ok := outputs[key]; ok {
			return output, nil
		}
		return "", errors.New("not available")
	}
	t.Cleanup(func() { runHintCommand = original })
	return &calls
}

func TestDetectEmailHintPrefersGit(t *testing.T) {
	calls := stubHintCommands(t, map[string]string{
		"git config --get user.email": "ada@example.com\n",
		"claude auth status":          "Logged in as other@example.com",
	})

	hint := detectEmailHint("claude_code")
	if hint == nil || hint.Email != "ada@example.com" || hint.Source != "git config user.email" {
		t.Fatalf("hint = %+v", hint)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected only git to be queried, got %v", *calls)
	}
}

func TestDetectEmailHintFallsBackToClaudeCode(t *testing.T) {
	stubHintCommands(t, map[string]string{
		"git config --get user.email": "12345+ada@users.noreply.github.com",
		"claude auth status":          "Login method: Claude Account\nEmail: Ada@Example.com\nOrg: Example",
	})

	hint := detectEmailHint("claude_code")
	if hint == nil || hint.Email != "Ada@Example.com" {
		t.Fatalf("hint = %+v", hint)
	}
}

func TestDetectEmailHintIgnoresClaudeForOtherAgents(t *testing.T) {
	calls := stubHintCommands(t, map[string]string{
		"claude auth status": "Email: ada@example.com",
	})

	if hint := detectEmailHint("cursor"); hint != nil {
		t.Fatalf("expected no hint, got %+v", hint)
	}
	for _, call := range *calls {
		if strings.HasPrefix(call, "claude") {
			t.Fatalf("claude must not be queried for other agents: %v", *calls)
		}
	}
}

func TestDetectEmailHintRejectsInvalidOutput(t *testing.T) {
	stubHintCommands(t, map[string]string{
		"git config --get user.email": "not an email",
		"claude auth status":          "Not logged in",
	})

	if hint := detectEmailHint("claude_code"); hint != nil {
		t.Fatalf("expected no hint, got %+v", hint)
	}
}

func TestWriteAgentInstructions(t *testing.T) {
	base := unclaimedProfile()

	t.Run("asks the user and suggests a detected email", func(t *testing.T) {
		profile := base
		profile.ClaimEmailHint = "ada@example.com"
		profile.ClaimEmailHintSource = "git config user.email"

		var out bytes.Buffer
		writeAgentInstructions(&out, "metorial-admin", profile)

		for _, want := range []string{
			"Ask them for the email address",
			"ada@example.com (found in git config user.email)",
			"confirm it with the user first",
			"metorial-admin claim <email>",
			claimlink.Identifier(base.ClaimToken),
		} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("instructions missing %q:\n%s", want, out.String())
			}
		}
	})

	t.Run("works without a hint", func(t *testing.T) {
		var out bytes.Buffer
		writeAgentInstructions(&out, "metorial-admin", base)

		if strings.Contains(out.String(), "likely address") {
			t.Fatalf("unexpected hint line:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "metorial-admin claim <email>") {
			t.Fatalf("missing claim command:\n%s", out.String())
		}
	})

	t.Run("stops asking for an email once the link was sent", func(t *testing.T) {
		profile := base
		profile.ClaimEmail = "ada@example.com"

		var out bytes.Buffer
		writeAgentInstructions(&out, "metorial-admin", profile)

		if !strings.Contains(out.String(), "A claim link was emailed to ada@example.com") {
			t.Fatalf("missing sent notice:\n%s", out.String())
		}
		if strings.Contains(out.String(), "Ask them for the email address") {
			t.Fatalf("should not ask for an email again:\n%s", out.String())
		}
	})
}

func TestSaveClaimEmailPersists(t *testing.T) {
	profile := unclaimedProfile()
	store := setupStore(t, profile)

	saveClaimEmail(store, profile.ID, "ada@example.com")

	reopened, _ := config.OpenStore()
	got, ok := reopened.ProfileByID(profile.ID)
	if !ok || got.ClaimEmail != "ada@example.com" {
		t.Fatalf("claim email not persisted: %+v", got)
	}
}

func TestUsableHintEmailRejectsMaskedAddresses(t *testing.T) {
	masked := []string{
		"12345+ada@users.noreply.github.com",
		"ada@noreply.github.com",
		"ada@users.noreply.gitlab.com",
		"ada@no-reply.example.com",
		"abc123@privaterelay.appleid.com",
		"ada.abc@mozmail.com",
		"ada@relay.firefox.com",
		"ada_at_example_com_xyz@duck.com",
		"ada.abc@simplelogin.com",
		"ada@slmail.me",
		"ada.abc@aleeas.com",
		"ada@mail.anonaddy.com",
		"ada@abc.anonaddy.me",
		"ada@addy.io",
		"ada@passmail.net",
		"ada@passinbox.com",
		"Ada <ada@DUCK.com>",
	}
	for _, candidate := range masked {
		if email, ok := usableHintEmail(candidate); ok {
			t.Errorf("%q should be rejected, got %q", candidate, email)
		}
	}

	real := []string{"ada@example.com", "ada@gmail.com", "ada@notduck.com", "ada@icloud.com"}
	for _, candidate := range real {
		if _, ok := usableHintEmail(candidate); !ok {
			t.Errorf("%q should be accepted", candidate)
		}
	}
}
