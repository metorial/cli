package consumerapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/config"
)

type recordedRequest struct {
	Method        string
	Path          string
	Query         url.Values
	Authorization string
	Headers       http.Header
	Body          map[string]any
}

func newTestClient(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Client, *[]recordedRequest) {
	t.Helper()

	requests := []recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := recordedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Query:         r.URL.Query(),
			Authorization: r.Header.Get("Authorization"),
			Headers:       r.Header.Clone(),
		}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &recorded.Body)
		}
		requests = append(requests, recorded)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	hostURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	client, err := New(config.Runtime{APIKey: "consumer-token", APIHost: server.URL, APIHostURL: hostURL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return client, &requests
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func TestListProvidersSendsFiltersWithoutImpersonationHeaders(t *testing.T) {
	client, requests := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []map[string]any{
				{"id": "cat_1", "type": "provider_template", "name": "GitHub", "availability": "available_now"},
			},
			"pagination": map[string]any{"has_more_after": true},
		})
	})

	result, err := client.ListProviders(ListOptions{Limit: 10, After: "cat_0"}, " git ", "grp_1")
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "cat_1" || !result.Pagination.HasMoreAfter {
		t.Fatalf("ListProviders() = %+v", result)
	}

	request := (*requests)[0]
	if request.Method != http.MethodGet || request.Path != "/consumer/providers" {
		t.Fatalf("request = %s %s", request.Method, request.Path)
	}
	if request.Query.Get("search") != "git" || request.Query.Get("provider_group_id") != "grp_1" {
		t.Fatalf("query = %v", request.Query)
	}
	if request.Query.Get("limit") != "10" || request.Query.Get("after") != "cat_0" {
		t.Fatalf("pagination query = %v", request.Query)
	}
	if !strings.Contains(request.Authorization, "consumer-token") {
		t.Fatalf("Authorization = %q", request.Authorization)
	}
	for _, header := range []string{"Metorial-Instance-Id", "Metorial-Consumer-Profile-Id"} {
		if value := request.Headers.Get(header); value != "" {
			t.Fatalf("%s = %q, consumer tokens should not send it", header, value)
		}
	}
}

func TestSetupDeployAndReconnectBodies(t *testing.T) {
	client, requests := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/setup"):
			writeJSON(w, http.StatusOK, map[string]any{"id": "iss_1", "status": "pending", "url": "https://setup.example/iss_1"})
		default:
			writeJSON(w, http.StatusOK, map[string]any{"id": "mms_1", "status": "active"})
		}
	})

	session, err := client.StartSetup("cat_1", "pam_oauth")
	if err != nil {
		t.Fatalf("StartSetup() error = %v", err)
	}
	if session.ID != "iss_1" || session.URL != "https://setup.example/iss_1" {
		t.Fatalf("StartSetup() = %+v", session)
	}

	name := "My GitHub"
	if _, err := client.Deploy("cat_1", DeployInput{IntegrationSetupSessionID: "iss_1", Name: &name}); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if _, err := client.Reconnect("cat_1", "mms_1", "iss_2"); err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}

	setup, deploy, reconnect := (*requests)[0], (*requests)[1], (*requests)[2]
	if setup.Method != http.MethodPost || setup.Path != "/consumer/providers/cat_1/setup" || setup.Body["provider_auth_method_id"] != "pam_oauth" {
		t.Fatalf("setup request = %+v", setup)
	}
	if deploy.Path != "/consumer/providers/cat_1/deploy" || deploy.Body["integration_setup_session_id"] != "iss_1" || deploy.Body["name"] != name {
		t.Fatalf("deploy request = %+v", deploy)
	}
	if _, ok := deploy.Body["description"]; ok {
		t.Fatalf("deploy request should omit an unset description: %+v", deploy.Body)
	}
	if reconnect.Path != "/consumer/providers/cat_1/reconnect" || reconnect.Body["magic_mcp_server_id"] != "mms_1" || reconnect.Body["integration_setup_session_id"] != "iss_2" {
		t.Fatalf("reconnect request = %+v", reconnect)
	}
}

func TestListMagicMcpServersOnlyRequestsActiveServers(t *testing.T) {
	client, requests := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "pagination": map[string]any{}})
	})

	if _, err := client.ListMagicMcpServers(ListOptions{}, "", "ptpl_1"); err != nil {
		t.Fatalf("ListMagicMcpServers() error = %v", err)
	}

	request := (*requests)[0]
	if request.Path != "/magic-mcp-servers" || request.Query.Get("status") != "active" || request.Query.Get("provider_template_id") != "ptpl_1" {
		t.Fatalf("request = %s %v", request.Path, request.Query)
	}
}

func TestErrorsMapToPortalHints(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		want    string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, message: "Invalid token", want: "metorial login"},
		{name: "payment required", status: http.StatusPaymentRequired, message: "Upgrade your plan", want: "not available on this organization's plan"},
		{name: "not entitled", status: http.StatusForbidden, message: "You are not entitled to access this endpoint", want: "not enabled for this organization"},
		{name: "other", status: http.StatusNotFound, message: "Catalog item not found", want: "Catalog item not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, tt.status, map[string]any{"status": "error", "code": "error", "message": tt.message})
			})

			_, err := client.GetProvider("cat_1")
			if err == nil {
				t.Fatalf("GetProvider() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("GetProvider() error = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}
