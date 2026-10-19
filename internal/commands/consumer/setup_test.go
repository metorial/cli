package consumer

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

func newTestCommand(t *testing.T, apiHost string, name string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	stdout := &bytes.Buffer{}
	features := terminal.Features{Width: 120}
	application := app.NewWithIO(bytes.NewReader(nil), stdout, &bytes.Buffer{}, features, features)
	application.SetSubjectType(config.SubjectTypeConsumer)

	ctx := commandutil.NewContext(application, &commandutil.RootOptions{
		APIKey:  "consumer-token",
		APIHost: apiHost,
		Format:  "json",
	})

	for _, command := range NewCommands(ctx) {
		if command.Name() == name {
			command.SetOut(stdout)
			command.SetErr(&bytes.Buffer{})
			return command, stdout
		}
	}

	t.Fatalf("command %q not found", name)
	return nil, nil
}

func TestSetupPollsUntilSuccessfulThenDeploys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previousInterval := setupPollInterval
	setupPollInterval = time.Millisecond
	t.Cleanup(func() { setupPollInterval = previousInterval })

	polls := 0
	calls := []string{}
	var deployBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/consumer/providers/cat_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                "cat_1",
				"type":              "provider_template",
				"name":              "GitHub",
				"availability":      "available_now",
				"provider_template": map[string]any{"id": "ptpl_1", "name": "GitHub"},
				"auth_methods": []map[string]any{
					{"id": "pam_token", "type": "token", "name": "Token"},
					{"id": "pam_oauth", "type": "oauth", "name": "OAuth"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/consumer/providers/cat_1/setup":
			body := map[string]any{}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			if body["provider_auth_method_id"] != "pam_oauth" {
				t.Errorf("setup should prefer the OAuth auth method, got %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "iss_1", "status": "pending"})
		case r.Method == http.MethodGet && r.URL.Path == "/consumer/providers/cat_1/setup/iss_1":
			polls++
			status := "pending"
			if polls >= 2 {
				status = "successful"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "iss_1", "status": status})
		case r.Method == http.MethodPost && r.URL.Path == "/consumer/providers/cat_1/deploy":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &deployBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":        "mms_1",
				"status":    "active",
				"name":      "Work GitHub",
				"endpoints": []map[string]any{{"id": "mmse_1", "alias": "work-github", "url": "https://mcp.example/work-github"}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "unexpected " + r.Method + " " + r.URL.Path})
		}
	}))
	defer server.Close()

	command, stdout := newTestCommand(t, server.URL, "setup")
	command.SetArgs([]string{"cat_1", "--name", "Work GitHub"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v (calls: %v)", err, calls)
	}

	if polls != 2 {
		t.Fatalf("polls = %d, want 2", polls)
	}
	if deployBody["integration_setup_session_id"] != "iss_1" || deployBody["name"] != "Work GitHub" {
		t.Fatalf("deploy body = %v", deployBody)
	}

	result := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout.String())
	}
	integration, _ := result["integration"].(map[string]any)
	if integration["id"] != "mms_1" {
		t.Fatalf("output integration = %v", result["integration"])
	}
}

func TestSetupStopsOnExpiredSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previousInterval := setupPollInterval
	setupPollInterval = time.Millisecond
	t.Cleanup(func() { setupPollInterval = previousInterval })

	deployed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/consumer/providers/cat_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cat_1", "type": "provider_template", "name": "GitHub", "availability": "available_now"})
		case strings.HasSuffix(r.URL.Path, "/setup"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "iss_1", "status": "pending"})
		case strings.HasSuffix(r.URL.Path, "/setup/iss_1"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "iss_1", "status": "expired"})
		case strings.HasSuffix(r.URL.Path, "/deploy"):
			deployed = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "mms_1"})
		}
	}))
	defer server.Close()

	command, _ := newTestCommand(t, server.URL, "setup")
	command.SetArgs([]string{"cat_1"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Execute() error = %v, want an expired session error", err)
	}
	if deployed {
		t.Fatalf("deploy should not be called for an expired setup session")
	}
}

func TestSetupRejectsRequestAccessItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/consumer/providers/cat_1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "cat_1", "type": "provider_template", "name": "Salesforce", "availability": "request_access"})
	}))
	defer server.Close()

	command, _ := newTestCommand(t, server.URL, "setup")
	command.SetArgs([]string{"cat_1"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "metorial request-access cat_1") {
		t.Fatalf("Execute() error = %v, want a request-access hint", err)
	}
}
