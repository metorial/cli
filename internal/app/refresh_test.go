package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metorial/cli/internal/config"
)

func TestConcurrentRefreshesRotateTheTokenOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvCredentialStorage, config.TokenStorageFile)
	t.Setenv(config.EnvAPIHost, "")
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvToken, "")

	var calls atomic.Int32
	var mu sync.Mutex
	current := "refresh_1"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		_ = request.ParseForm()
		mu.Lock()
		defer mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if request.PostForm.Get("refresh_token") != current {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":"invalid_grant"}`)
			return
		}
		current = "refresh_2"
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"access_token":  "access_2",
			"refresh_token": "refresh_2",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProfile(config.Profile{
		Name:              "Ada",
		APIHost:           server.URL,
		ClientID:          "client_1",
		AccessToken:       "access_1",
		RefreshToken:      "refresh_1",
		ExpiresAt:         time.Now().UTC().Add(-time.Minute),
		SubjectType:       config.SubjectTypeConsumer,
		InstanceID:        "ins_1",
		ConsumerProfileID: "cpr_1",
	}, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	var wait sync.WaitGroup
	tokens := make([]string, 4)
	errs := make([]error, 4)
	for index := range tokens {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			app := New()
			app.SetSubjectType(config.SubjectTypeConsumer)
			runtime, err := app.ResolveAuthConfig("", "", "")
			tokens[index], errs[index] = runtime.APIKey, err
		}(index)
	}
	wait.Wait()

	for index := range tokens {
		if errs[index] != nil || tokens[index] != "access_2" {
			t.Fatalf("caller %d got %q, %v; want access_2", index, tokens[index], errs[index])
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func signedInConsumer(t *testing.T, apiHost string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvCredentialStorage, config.TokenStorageFile)
	t.Setenv(config.EnvAPIHost, "")
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvToken, "")

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProfile(config.Profile{
		Name:              "Ada",
		APIHost:           apiHost,
		ClientID:          "client_1",
		AccessToken:       "access_1",
		RefreshToken:      "refresh_1",
		ExpiresAt:         time.Now().UTC().Add(-time.Minute),
		SubjectType:       config.SubjectTypeConsumer,
		InstanceID:        "ins_1",
		ConsumerProfileID: "cpr_1",
	}, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
}

func consumerApp() *App {
	app := New()
	app.SetSubjectType(config.SubjectTypeConsumer)
	return app
}

func rejectRefresh(onRefresh func()) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		onRefresh()
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":"invalid_grant"}`)
	}))
}

func TestARefreshTokenRotatedElsewhereIsAdoptedAfterInvalidGrant(t *testing.T) {
	server := rejectRefresh(func() {
		store, _ := config.OpenStore()
		profile, _ := store.ProfileByID("ins_1:cpr_1")
		profile.AccessToken = "elsewhere"
		profile.RefreshToken = "refresh_elsewhere"
		profile.ExpiresAt = time.Now().UTC().Add(time.Hour)
		_ = store.UpdateProfile(*profile, true)
	})
	defer server.Close()
	signedInConsumer(t, server.URL)

	runtime, err := consumerApp().ResolveAuthConfig("", "", "")
	if err != nil || runtime.APIKey != "elsewhere" {
		t.Fatalf("ResolveAuthConfig() = %q, %v; want the token rotated elsewhere", runtime.APIKey, err)
	}
}

func TestARefreshDoesNotBringBackAProfileRemovedMeanwhile(t *testing.T) {
	served := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path, _ := config.DefaultConfigPath()
		content, _ := os.ReadFile(path)
		raw := map[string]any{}
		_ = json.Unmarshal(content, &raw)
		delete(raw["profiles"].(map[string]any), "ins_1:cpr_1")
		content, _ = json.Marshal(raw)
		_ = os.WriteFile(path, content, 0o600)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"access_2","refresh_token":"refresh_2","expires_in":3600}`)
	}))
	defer served.Close()
	signedInConsumer(t, served.URL)

	if _, err := consumerApp().ResolveAuthConfig("", "", ""); err == nil {
		t.Fatalf("ResolveAuthConfig() succeeded for a profile removed during its refresh")
	}
	store, _ := config.OpenStore()
	if _, ok := store.ProfileByID("ins_1:cpr_1"); ok {
		t.Fatalf("the refresh recreated the removed profile")
	}
}

func TestProfileTokensAreNeverSentToAnotherHost(t *testing.T) {
	signedInConsumer(t, "https://api.metorial.com")
	store, _ := config.OpenStore()
	profile, _ := store.ProfileByID("ins_1:cpr_1")
	profile.ExpiresAt = time.Now().UTC().Add(time.Hour)
	_ = store.UpdateProfile(*profile, true)

	if _, err := consumerApp().ResolveAuthConfig("", "https://evil.example", ""); err == nil {
		t.Fatalf("ResolveAuthConfig() used the profile's tokens for another host")
	}
	t.Setenv(config.EnvAPIHost, "https://evil.example")
	if _, err := consumerApp().ResolveAuthConfig("", "", ""); err == nil {
		t.Fatalf("ResolveAuthConfig() used the profile's tokens for METORIAL_API_HOST")
	}
	runtime, err := consumerApp().ResolveAuthConfig("key_1", "https://evil.example", "")
	if err != nil || runtime.APIKey != "key_1" {
		t.Fatalf("an explicit API key = %q, %v; want it to keep working", runtime.APIKey, err)
	}
	t.Setenv(config.EnvAPIHost, "api.metorial.com")
	if runtime, err := consumerApp().ResolveAuthConfig("", "", ""); err != nil || runtime.APIKey != "access_1" {
		t.Fatalf("the profile's own host = %q, %v", runtime.APIKey, err)
	}
}
