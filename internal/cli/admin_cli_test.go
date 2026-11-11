package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestAdminProviderDiscoveryUsesCatalog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("METORIAL_SKIP_UPDATE_CHECK", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider-listings" || r.URL.Query().Get("search") != "github" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"id":"plg_1","provider":{"id":"pro_1"}}]}`)
	}))
	defer server.Close()

	output, err := executeAdmin(t, "--api-host", server.URL, "--api-key", "metorial_sk_test",
		"--format", "json", "providers", "list", "--search", "github")
	var result struct {
		Items []struct {
			Provider struct{ ID string } `json:"provider"`
		}
	}
	if err != nil || json.Unmarshal([]byte(output), &result) != nil ||
		len(result.Items) != 1 || result.Items[0].Provider.ID != "pro_1" {
		t.Fatalf("providers list = %q, %v", output, err)
	}
}

func TestAdminIntegrationSetupResumesByID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("METORIAL_SKIP_UPDATE_CHECK", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/integration-setup-sessions":
			_, _ = io.WriteString(w, `{"id":"iss_1","status":"pending","url":"https://example.test/setup"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/integration-setup-sessions/iss_1":
			_, _ = io.WriteString(w, `{"id":"iss_1","status":"successful","integration_instance":{"id":"ini_1"}}`)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	base := []string{"--api-host", server.URL, "--api-key", "metorial_sk_test", "--format", "json", "integrations", "setup"}
	for _, step := range []struct {
		args   []string
		status string
	}{
		{[]string{"--integration-id", "int_1", "--name", "GitHub", "--no-browser"}, "pending"},
		{[]string{"status", "iss_1"}, "successful"},
		{[]string{"wait", "iss_1", "--timeout", "1s"}, "successful"},
	} {
		output, err := executeAdmin(t, append(base, step.args...)...)
		var result struct{ ID, Status string }
		if err != nil || json.Unmarshal([]byte(output), &result) != nil || result.ID != "iss_1" || result.Status != step.status {
			t.Fatalf("setup %v = %q, %v", step.args, output, err)
		}
	}
}

func TestAdminPersonalIntegrationReusesConsumerSetupAndMCP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("METORIAL_SKIP_UPDATE_CHECK", "1")

	protocol := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	protocol.AddTool(&mcp.Tool{Name: "example", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			t.Error("verification must not call tools")
			return nil, nil
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return protocol }, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if r.URL.Path == "/member-consumer-session" {
			if r.Method != http.MethodPost || authorization != "Bearer admin-token" ||
				r.Header.Get("metorial-instance-id") != "ins_1" {
				http.Error(w, "invalid exchange request", http.StatusUnauthorized)
				return
			}
			var body struct {
				IntegrationID string `json:"integration_id"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				http.Error(w, "invalid exchange body", http.StatusBadRequest)
				return
			}
			response := map[string]string{"access_token": "ephemeral-consumer-token"}
			if body.IntegrationID == "int_1" {
				response["catalog_item_id"] = "ptb_1"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		if authorization != "Bearer ephemeral-consumer-token" {
			http.Error(w, "consumer credential required", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/connect/magic/example" {
			mcpHandler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /consumer/providers/ptb_1":
			_, _ = io.WriteString(w, `{"id":"cat_1","type":"provider_template","availability":"available_now"}`)
		case "POST /consumer/providers/cat_1/setup":
			_, _ = io.WriteString(w, `{"id":"iss_1","status":"pending","url":"https://example.test/setup"}`)
		case "GET /consumer/providers/cat_1/setup/iss_1":
			_, _ = io.WriteString(w, `{"id":"iss_1","status":"successful"}`)
		case "POST /consumer/providers/cat_1/deploy", "GET /magic-mcp-servers/mgsr_1":
			_, _ = fmt.Fprintf(w, `{"id":"mgsr_1","endpoints":[{"url":"http://%s/connect/magic/example"}]}`, r.Host)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	base := []string{"--api-host", server.URL, "--api-key", "admin-token", "--instance", "ins_1",
		"--format", "json", "integrations", "personal"}
	for _, step := range []struct {
		args []string
		want string
	}{
		{[]string{"setup", "int_1", "--no-browser"}, `"status": "pending"`},
		{[]string{"status", "cat_1", "iss_1"}, `"status": "successful"`},
		{[]string{"wait", "cat_1", "iss_1"}, `"status": "successful"`},
		{[]string{"finish", "cat_1", "iss_1"}, `"tool_count": 1`},
		{[]string{"tools", "mgsr_1"}, `"name": "example"`},
	} {
		output, err := executeAdmin(t, append(base, step.args...)...)
		if err != nil || !strings.Contains(output, step.want) || strings.Contains(output, "ephemeral-consumer-token") {
			t.Fatalf("personal %v = %q, %v", step.args, output, err)
		}
	}
}
