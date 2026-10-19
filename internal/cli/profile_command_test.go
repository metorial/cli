package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
)

func testConsumerProfile(instanceID, consumerProfileID, name, portalName string) config.Profile {
	return config.Profile{
		ID:                config.ProfileID(instanceID, consumerProfileID),
		Name:              name,
		SubjectType:       config.SubjectTypeConsumer,
		ConsumerProfileID: consumerProfileID,
		ConsumerName:      name,
		ConsumerEmail:     name + "@example.com",
		PortalID:          "ptl_" + name,
		PortalName:        portalName,
		InstanceID:        instanceID,
		APIHost:           "https://api.metorial.com",
		ExpiresAt:         time.Now().UTC().Add(time.Hour),
	}
}

func testMemberProfile(orgID, userID, name string) config.Profile {
	return config.Profile{
		ID:        config.ProfileID(orgID, userID),
		Name:      name,
		OrgID:     orgID,
		OrgName:   name + " Org",
		UserID:    userID,
		UserEmail: name + "@example.com",
		APIHost:   "https://api.metorial.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func runConsumerRoot(t *testing.T, args ...string) string {
	t.Helper()

	stdout := &bytes.Buffer{}
	command, err := NewConsumerRootCommand(&app.App{
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("NewConsumerRootCommand() error = %v", err)
	}

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}

	return stdout.String()
}

func TestProfileSetAcceptsName(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.UpsertProfile(testConsumerProfile("ins_1", "cpr_1", "alpha", "Alpha Portal"), true); err != nil {
		t.Fatalf("UpsertProfile(first) error = %v", err)
	}
	if err := store.UpsertProfile(testConsumerProfile("ins_2", "cpr_2", "beta", "Beta Portal"), false); err != nil {
		t.Fatalf("UpsertProfile(second) error = %v", err)
	}

	rendered := runConsumerRoot(t, "profile", "set", "beta")

	reloaded, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() reload error = %v", err)
	}

	current, ok := reloaded.CurrentProfileFor(config.SubjectTypeConsumer)
	if !ok {
		t.Fatalf("CurrentProfileFor(consumer) missing")
	}
	if current.Name != "beta" {
		t.Fatalf("CurrentProfileFor(consumer).Name = %q, want %q", current.Name, "beta")
	}

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

	profile := testConsumerProfile("ins_1", "cpr_1", "demo", "Demo Portal")
	if err := store.UpsertProfile(profile, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	rendered := runConsumerRoot(t, "profile", "list")
	if !strings.Contains(rendered, "These are the profiles you are currently logged in with.") {
		t.Fatalf("list output = %q", rendered)
	}
	if strings.Contains(rendered, profile.ID) {
		t.Fatalf("list output should not include profile id: %q", rendered)
	}
	if !strings.Contains(rendered, "Demo Portal") {
		t.Fatalf("list output missing portal: %q", rendered)
	}
	if !strings.Contains(rendered, "`metorial profile set demo`") {
		t.Fatalf("list output missing example: %q", rendered)
	}
}

func TestProfileListHidesMemberProfiles(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.UpsertProfile(testMemberProfile("org_1", "usr_1", "admin"), true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	rendered := runConsumerRoot(t, "profile", "list")
	if !strings.Contains(rendered, "No saved profiles.") {
		t.Fatalf("list output should not include member profiles: %q", rendered)
	}
}

func TestProfileGetIncludesSwitchExample(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.UpsertProfile(testConsumerProfile("ins_1", "cpr_1", "demo", "Demo Portal"), true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	rendered := runConsumerRoot(t, "profile", "get", "demo")
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
