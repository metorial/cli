package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
)

func executeAdmin(t *testing.T, args ...string) (string, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	command, err := NewAdminRootCommand(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("NewAdminRootCommand() error = %v", err)
	}

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err = command.Execute()
	return stdout.String(), err
}

func TestAdminRootHelpIncludesAgentDiscovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CURSOR_AGENT", "1")

	rendered, err := executeAdmin(t, "--help")
	if err != nil {
		t.Fatalf("help error = %v", err)
	}

	for _, text := range []string{
		"=== AGENT COMMAND DISCOVERY ===",
		"metorial-admin --format json cli search",
		"Every word must match",
		"Run the selected command with --help",
	} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("help output missing %q:\n%s", text, rendered)
		}
	}
}

func TestAdminRootHelpOmitsAgentDiscoveryForPeople(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{
		"ANTIGRAVITY_CLI_ALIAS", "CLAUDECODE", "CLINE_ACTIVE", "CODEX_SANDBOX",
		"CODEX_THREAD_ID", "CODEX_SANDBOX_NETWORK_DISABLED", "CODEX_CI",
		"CURSOR_AGENT", "GEMINI_CLI", "OPENCODE",
	} {
		t.Setenv(name, "")
	}

	rendered, err := executeAdmin(t, "--help")
	if err != nil {
		t.Fatalf("help error = %v", err)
	}
	if strings.Contains(rendered, "=== AGENT COMMAND DISCOVERY ===") {
		t.Fatalf("help output unexpectedly includes agent guidance:\n%s", rendered)
	}
}

func TestAdminCLISearchListsMatchingCommands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rendered, err := executeAdmin(t, "cli", "search", "profile")
	if err != nil {
		t.Fatalf("cli search error = %v", err)
	}

	if !strings.Contains(rendered, "Matching Commands") {
		t.Fatalf("search output = %q", rendered)
	}
	if !strings.Contains(rendered, "metorial-admin cli profile set") {
		t.Fatalf("search output missing profile set: %q", rendered)
	}
	if !strings.Contains(rendered, "Set the current profile") {
		t.Fatalf("search output missing description: %q", rendered)
	}
}

func TestAdminCLISearchJSONRequiresAllTerms(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rendered, err := executeAdmin(t, "--format", "json", "cli", "search", "settings", "unset")
	if err != nil {
		t.Fatalf("cli search error = %v", err)
	}

	var result struct {
		Commands []struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput: %s", err, rendered)
	}

	if len(result.Commands) != 1 || result.Commands[0].Command != "metorial-admin cli settings unset" {
		t.Fatalf("commands = %#v", result.Commands)
	}
}

func TestAdminCLISearchReportsNoMatches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rendered, err := executeAdmin(t, "cli", "search", "definitely-not-a-command")
	if err != nil {
		t.Fatalf("cli search error = %v", err)
	}

	if !strings.Contains(rendered, "No Matching Commands") {
		t.Fatalf("search output = %q", rendered)
	}
}

func TestAdminProfileAndSettingsLiveUnderCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, name := range []string{"profile", "settings"} {
		command, err := NewAdminRootCommand(&app.App{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
		if err != nil {
			t.Fatalf("NewAdminRootCommand() error = %v", err)
		}

		if found, _, err := command.Find([]string{name}); err == nil && found != command {
			t.Fatalf("top-level %q should not exist", name)
		}
		if found, _, err := command.Find([]string{"cli", name}); err != nil || found.Name() != name {
			t.Fatalf("cli %s missing: %v", name, err)
		}
	}
}

func TestAdminCLIProfileHintsUseAdminPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProfile(config.Profile{
		ID:        config.ProfileID("org_1", "usr_1"),
		Name:      "demo",
		OrgID:     "org_1",
		UserID:    "usr_1",
		APIHost:   "https://api.metorial.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	rendered, err := executeAdmin(t, "cli", "profile", "list")
	if err != nil {
		t.Fatalf("cli profile list error = %v", err)
	}

	if !strings.Contains(rendered, "`metorial-admin cli profile set demo`") {
		t.Fatalf("list output missing admin hint: %q", rendered)
	}
}
