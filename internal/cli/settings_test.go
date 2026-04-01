package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/app"
)

func TestNewRootCommandUsesSavedDefaultFormat(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	settingsCommand := newSettingsCommand(&app.App{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, &rootOptions{format: "structured"})
	settingsCommand.SetOut(&bytes.Buffer{})
	settingsCommand.SetErr(&bytes.Buffer{})
	settingsCommand.SetArgs([]string{"set", "default-format", "json"})
	if err := settingsCommand.Execute(); err != nil {
		t.Fatalf("settings set default-format error = %v", err)
	}

	command, err := newRootCommand(&app.App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("newRootCommand() error = %v", err)
	}

	flag := command.PersistentFlags().Lookup("format")
	if flag == nil {
		t.Fatalf("format flag missing")
	}
	if flag.DefValue != "json" {
		t.Fatalf("format flag default = %q, want %q", flag.DefValue, "json")
	}
}

func TestSettingsListShowsDefaultFormat(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	command := newSettingsCommand(&app.App{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, &rootOptions{format: "structured"})
	command.SetErr(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"set", "default-format", "toml"})
	if err := command.Execute(); err != nil {
		t.Fatalf("settings set default-format error = %v", err)
	}

	stdout := &bytes.Buffer{}
	command = newSettingsCommand(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}}, &rootOptions{format: "structured"})
	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("settings list error = %v", err)
	}

	if !strings.Contains(stdout.String(), "default_format = \"toml\"") {
		t.Fatalf("settings list output = %q", stdout.String())
	}
}

func TestSettingsSetAcceptsUnderscoreNames(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	command := newSettingsCommand(&app.App{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, &rootOptions{format: "structured"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"set", "default_format", "json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("settings set default_format error = %v", err)
	}

	stdout := &bytes.Buffer{}
	command = newSettingsCommand(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}}, &rootOptions{format: "structured"})
	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("settings list error = %v", err)
	}

	if !strings.Contains(stdout.String(), "default_format = \"json\"") {
		t.Fatalf("settings list output = %q", stdout.String())
	}
}
