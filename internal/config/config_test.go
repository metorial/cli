package config

import (
	"testing"
	"time"
)

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "bare host", input: "api.metorial.com", want: "https://api.metorial.com"},
		{name: "full url", input: "https://api.metorial.com", want: "https://api.metorial.com"},
		{name: "trim path", input: "https://api.metorial.com/v1?x=1", want: "https://api.metorial.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeBaseURL(tt.input)
			if err != nil {
				t.Fatalf("NormalizeBaseURL() error = %v", err)
			}

			if got.String() != tt.want {
				t.Fatalf("NormalizeBaseURL() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestStoreRoundTrip(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	profile := Profile{
		ID:           ProfileID("org_123", "usr_456"),
		Name:         "demo-org",
		APIHost:      "https://api.metorial.com",
		ClientID:     "client_123",
		AccessToken:  "access_123",
		RefreshToken: "refresh_123",
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
		OrgID:        "org_123",
		OrgName:      "Demo Org",
		UserID:       "usr_456",
		UserEmail:    "demo@example.com",
	}

	if err := store.UpsertProfile(profile, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	reloaded, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() reload error = %v", err)
	}

	current, ok := reloaded.CurrentProfile()
	if !ok {
		t.Fatalf("CurrentProfile() = missing, want present")
	}

	if current.ID != profile.ID {
		t.Fatalf("CurrentProfile().ID = %q, want %q", current.ID, profile.ID)
	}

	if current.AccessToken != profile.AccessToken {
		t.Fatalf("CurrentProfile().AccessToken = %q, want %q", current.AccessToken, profile.AccessToken)
	}

	if len(reloaded.SortedProfiles()) != 1 {
		t.Fatalf("SortedProfiles() len = %d, want 1", len(reloaded.SortedProfiles()))
	}

	if err := reloaded.UpdateSettings(func(settings *Settings) {
		settings.DefaultAPIHost = "https://api.example.com"
		settings.DefaultFormat = "json"
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	reloadedAfterSettings, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() after settings error = %v", err)
	}

	if reloadedAfterSettings.Settings().DefaultAPIHost != "https://api.example.com" {
		t.Fatalf("Settings().DefaultAPIHost = %q, want %q", reloadedAfterSettings.Settings().DefaultAPIHost, "https://api.example.com")
	}
	if reloadedAfterSettings.Settings().DefaultFormat != "json" {
		t.Fatalf("Settings().DefaultFormat = %q, want %q", reloadedAfterSettings.Settings().DefaultFormat, "json")
	}
}

func TestResolveAPIHostWithDefault(t *testing.T) {
	t.Parallel()

	got, err := ResolveAPIHostWithDefault("", "https://api.example.com")
	if err != nil {
		t.Fatalf("ResolveAPIHostWithDefault() error = %v", err)
	}

	if got.String() != "https://api.example.com" {
		t.Fatalf("ResolveAPIHostWithDefault() = %q, want %q", got.String(), "https://api.example.com")
	}
}

func TestProjectStoreRoundTrip(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()

	store, err := OpenProjectStore(projectDir)
	if err != nil {
		t.Fatalf("OpenProjectStore() error = %v", err)
	}

	if err := store.SetSelectedInstance("profile:https://api.metorial.com:org:user", "inst_123"); err != nil {
		t.Fatalf("SetSelectedInstance() error = %v", err)
	}

	reloaded, err := OpenProjectStore(projectDir)
	if err != nil {
		t.Fatalf("OpenProjectStore() reload error = %v", err)
	}

	instanceID, ok := reloaded.SelectedInstance("profile:https://api.metorial.com:org:user")
	if !ok {
		t.Fatalf("SelectedInstance() = missing, want present")
	}

	if instanceID != "inst_123" {
		t.Fatalf("SelectedInstance() = %q, want %q", instanceID, "inst_123")
	}
}
