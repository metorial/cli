package fetch

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/config"
)

func TestResolveURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		host    string
		target  string
		want    string
		wantErr bool
	}{
		{name: "path", host: "https://api.metorial.com", target: "/provider-listings", want: "https://api.metorial.com/provider-listings"},
		{name: "path without slash", host: "https://api.metorial.com", target: "provider-listings", want: "https://api.metorial.com/provider-listings"},
		{name: "matching full url", host: "https://api.metorial.com", target: "https://api.metorial.com/provider-listings", want: "https://api.metorial.com/provider-listings"},
		{name: "mismatched host", host: "https://api.metorial.com", target: "https://example.com/provider-listings", wantErr: true},
		{name: "base path", host: "https://metorial.acme.com/v1", target: "/provider-listings", want: "https://metorial.acme.com/v1/provider-listings"},
		{name: "base path with query", host: "https://metorial.acme.com/v1", target: "/provider-listings?limit=5&cursor=a%2Fb", want: "https://metorial.acme.com/v1/provider-listings?limit=5&cursor=a%2Fb"},
		{name: "base path with escaped segment", host: "https://metorial.acme.com/v1", target: "/files/a%2Fb", want: "https://metorial.acme.com/v1/files/a%2Fb"},
		{name: "base path matching full url", host: "https://metorial.acme.com/v1", target: "https://metorial.acme.com/v1/provider-listings", want: "https://metorial.acme.com/v1/provider-listings"},
		{name: "base path full url with another scheme", host: "https://metorial.acme.com/v1", target: "http://metorial.acme.com/v1/provider-listings", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			apiHost, err := config.NormalizeBaseURL(tt.host)
			if err != nil {
				t.Fatalf("NormalizeBaseURL() error = %v", err)
			}

			got, err := ResolveURL(apiHost, tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveURL() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveURL() error = %v", err)
			}

			if got.String() != tt.want {
				t.Fatalf("ResolveURL() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestDoRetriesOnceWithRefreshedCredentials(t *testing.T) {
	t.Parallel()

	authorizations := []string{}
	bodies := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if strings.Contains(r.Header.Get("Authorization"), "expired") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hostURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	runtime := config.Runtime{APIKey: "expired", APIHost: server.URL, APIHostURL: hostURL}
	runtime.Refresh = func(bool) (config.Runtime, error) {
		refreshed := runtime
		refreshed.APIKey = "fresh"
		refreshed.Refresh = nil
		return refreshed, nil
	}

	response, err := Do(runtime, Request{
		Method:  http.MethodPost,
		Target:  "/files",
		Headers: map[string]string{"Content-Type": "text/plain"},
		Body:    []byte("payload"),
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Do() status = %d, want 200", response.StatusCode)
	}
	if len(authorizations) != 2 || !strings.Contains(authorizations[1], "fresh") {
		t.Fatalf("authorizations = %v, want a retry with the refreshed key", authorizations)
	}
	if bodies[1] != "payload" {
		t.Fatalf("retried body = %q, want the original payload", bodies[1])
	}
}
