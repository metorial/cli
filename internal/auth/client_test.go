package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return NewClient(baseURL)
}

func TestCLISignupDecodesOrganizationIntoTokenResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli/onboarding/signup" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["organization_name"] != "tobias's workspace" {
			t.Errorf("organization_name = %q", body["organization_name"])
		}
		if _, ok := body["region"]; ok {
			t.Errorf("region must be omitted when empty, got %q", body["region"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "at", "refresh_token": "rt", "client_id": "cli", "subject_type": "organization_member",
			"user": {"id": "usr_1", "name": "CLI", "email": "cli@example.com"},
			"organization": {"id": "org_1", "name": "Org", "slug": "brave-otter"},
			"project": {"id": "prj_1", "name": "Default", "slug": "default"},
			"instance": {"id": "ins_1", "name": "Production", "slug": "production"},
			"claim": {"status": "unclaimed", "token": "mt_claim_x_ext", "url": "https://example.com/claim"}
		}`))
	})

	response, err := client.CLISignup("tobias's workspace", "")
	if err != nil {
		t.Fatalf("CLISignup() error = %v", err)
	}
	if response.TokenResponse.Organization.ID != "org_1" || response.Organization.Slug != "brave-otter" {
		t.Fatalf("organization not decoded: %+v", response)
	}
	if response.TokenResponse.Instance == nil || response.TokenResponse.Instance.ID != "ins_1" {
		t.Fatalf("instance not copied into token response: %+v", response.TokenResponse.Instance)
	}
	if response.Claim.Token != "mt_claim_x_ext" || response.User.ID != "usr_1" {
		t.Fatalf("claim/user not decoded: %+v", response)
	}
}

func TestServiceErrorCodeIsMapped(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status": 429, "code": "claim_email_org_limit", "message": "Too many orgs."}`))
	})

	_, err := client.CLIClaimEmail("tok", "org_1", "claim", "a@example.com")
	apiError := &Error{}
	if !errors.As(err, &apiError) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if apiError.ErrorCode != "claim_email_org_limit" || apiError.ErrorMessage != "Too many orgs." {
		t.Fatalf("unexpected error mapping: %+v", apiError)
	}
}

func TestCLISignupSendsRegionAndDecodesIt(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["region"] != "eu1" {
			t.Errorf("region = %q", body["region"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "at", "organization": {"id": "org_1"}, "region": "eu1"}`))
	})

	response, err := client.CLISignup("", " eu1 ")
	if err != nil {
		t.Fatalf("CLISignup() error = %v", err)
	}
	if response.Region != "eu1" {
		t.Fatalf("Region = %q", response.Region)
	}
}

func TestCLISignupRegionsDecodesRecommendation(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli/onboarding/regions" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"regions": [
				{"id": "us1", "name": "United States", "trace_url": "https://api-us1.metorial.com/.metorial/trace-region"},
				{"id": "eu1", "name": "European Union", "trace_url": "https://api-eu1.metorial.com/.metorial/trace-region"}
			],
			"recommended_region": null
		}`))
	})

	response, err := client.CLISignupRegions()
	if err != nil {
		t.Fatalf("CLISignupRegions() error = %v", err)
	}
	if len(response.Regions) != 2 || response.Regions[1].TraceURL == "" || response.RecommendedRegion != "" {
		t.Fatalf("regions not decoded: %+v", response)
	}
}
