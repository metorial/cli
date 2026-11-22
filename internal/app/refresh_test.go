package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
