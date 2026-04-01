package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
)

func TestProfileSetAcceptsName(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	first := config.Profile{
		ID:        config.ProfileID("org_1", "usr_1"),
		Name:      "alpha",
		OrgID:     "org_1",
		OrgName:   "Alpha Org",
		UserID:    "usr_1",
		UserEmail: "alpha@example.com",
		APIHost:   "https://api.metorial.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	second := config.Profile{
		ID:        config.ProfileID("org_2", "usr_2"),
		Name:      "beta",
		OrgID:     "org_2",
		OrgName:   "Beta Org",
		UserID:    "usr_2",
		UserEmail: "beta@example.com",
		APIHost:   "https://api.metorial.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	if err := store.UpsertProfile(first, true); err != nil {
		t.Fatalf("UpsertProfile(first) error = %v", err)
	}
	if err := store.UpsertProfile(second, false); err != nil {
		t.Fatalf("UpsertProfile(second) error = %v", err)
	}

	stdout := &bytes.Buffer{}
	command, err := newRootCommand(&app.App{
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("newRootCommand() error = %v", err)
	}

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"profile", "set", "beta"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	reloaded, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() reload error = %v", err)
	}

	current, ok := reloaded.CurrentProfile()
	if !ok {
		t.Fatalf("CurrentProfile() missing")
	}
	if current.Name != "beta" {
		t.Fatalf("CurrentProfile().Name = %q, want %q", current.Name, "beta")
	}

	rendered := stdout.String()
	if !strings.Contains(rendered, "Profile Updated") {
		t.Fatalf("set output = %q", rendered)
	}
	if !strings.Contains(rendered, "Active profile: beta") {
		t.Fatalf("set output = %q", rendered)
	}
	if !strings.Contains(rendered, "`metorial --help`") {
		t.Fatalf("set output missing help hint: %q", rendered)
	}
}

func TestProfileListIncludesFriendlyHelp(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	profile := config.Profile{
		ID:        config.ProfileID("org_1", "usr_1"),
		Name:      "demo",
		OrgID:     "org_1",
		OrgName:   "Demo Org",
		UserID:    "usr_1",
		UserEmail: "demo@example.com",
		APIHost:   "https://api.metorial.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := store.UpsertProfile(profile, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	command, err := newRootCommand(&app.App{
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("newRootCommand() error = %v", err)
	}

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"profile", "list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rendered := stdout.String()
	if !strings.Contains(rendered, "These are the profiles you are currently logged in with.") {
		t.Fatalf("list output = %q", rendered)
	}
	if strings.Contains(rendered, profile.ID) {
		t.Fatalf("list output should not include profile id: %q", rendered)
	}
	if !strings.Contains(rendered, "`metorial profile set demo`") {
		t.Fatalf("list output missing example: %q", rendered)
	}
}

func TestProfileGetIncludesSwitchExample(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	profile := config.Profile{
		ID:        config.ProfileID("org_1", "usr_1"),
		Name:      "demo",
		OrgID:     "org_1",
		OrgName:   "Demo Org",
		UserID:    "usr_1",
		UserEmail: "demo@example.com",
		APIHost:   "https://api.metorial.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := store.UpsertProfile(profile, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	command, err := newRootCommand(&app.App{
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("newRootCommand() error = %v", err)
	}

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"profile", "get", "demo"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rendered := stdout.String()
	if !strings.Contains(rendered, "This is one of the profiles you are currently logged in with.") {
		t.Fatalf("get output = %q", rendered)
	}
	if !strings.Contains(rendered, "Name: demo") {
		t.Fatalf("get output missing name: %q", rendered)
	}
	if !strings.Contains(rendered, "`metorial profile set demo`") {
		t.Fatalf("get output missing example: %q", rendered)
	}
}
