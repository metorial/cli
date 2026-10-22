package skills

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/skillsapi/skillstest"
	"github.com/metorial/cli/internal/terminal"
)

func runSkills(t *testing.T, fake *skillstest.Server, stdin string, args ...string) error {
	t.Helper()

	features := terminal.Features{Width: 120}
	application := app.NewWithIO(strings.NewReader(stdin), &bytes.Buffer{}, &bytes.Buffer{}, features, features)
	application.SetSubjectType(config.SubjectTypeConsumer)

	command := NewCommand(commandutil.NewContext(application, &commandutil.RootOptions{
		APIKey:  "test-token",
		APIHost: fake.URL,
		Format:  "json",
	}))
	command.SetIn(strings.NewReader(stdin))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	return command.Execute()
}

func TestFilesPutMoveRemoveHonorIfMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/notes.md", "notes")

	if err := runSkills(t, fake, "new", "files", "put", skillstest.SkillSlug, "/notes.md", "--if-match", "absent"); err == nil {
		t.Fatal("put --if-match absent should fail for an existing file")
	}
	if err := runSkills(t, fake, "", "files", "mv", skillstest.SkillSlug, "/notes.md", "/docs/notes.md", "--if-match", "stale"); err == nil {
		t.Fatal("mv with a stale etag should fail")
	}
	if err := runSkills(t, fake, "", "files", "mv", skillstest.SkillSlug, "/notes.md", "/docs/notes.md"); err != nil {
		t.Fatalf("mv error = %v", err)
	}
	if content, _ := fake.Content("/docs/notes.md"); content != "notes" {
		t.Fatalf("moved content = %q", content)
	}
	if err := runSkills(t, fake, "", "files", "rm", skillstest.SkillSlug, "/docs/notes.md", "--if-match", " absent "); err == nil || !strings.Contains(err.Error(), "only applies to writes") {
		t.Fatalf("rm --if-match absent error = %v", err)
	}
	if err := runSkills(t, fake, "", "files", "rm", skillstest.SkillSlug, "/docs/notes.md"); err != nil {
		t.Fatalf("rm error = %v", err)
	}
	if _, exists := fake.Content("/docs/notes.md"); exists {
		t.Fatal("notes.md should be deleted")
	}
}

func TestFilesPutUploadsBinaryWithConsumerToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	if err := runSkills(t, fake, "\x89PNG\x00binary", "files", "put", skillstest.SkillSlug, "/assets/logo.png"); err != nil {
		t.Fatalf("put error = %v", err)
	}
	if fake.Uploads != 1 || len(fake.StorageErrors) != 0 {
		t.Fatalf("uploads = %d, storage errors = %v; want one signed upload", fake.Uploads, fake.StorageErrors)
	}
	if content, _ := fake.Content("/assets/logo.png"); content != "\x89PNG\x00binary" {
		t.Fatalf("remote logo.png = %q", content)
	}
}
