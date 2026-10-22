package skillsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/skillsapi/skillstest"
)

func pullTestSkill(t *testing.T, fake *skillstest.Server) string {
	t.Helper()

	client := fake.Client(t)
	skill, err := client.ResolveSkill(skillstest.SkillSlug)
	if err != nil {
		t.Fatalf("ResolveSkill() error = %v", err)
	}

	dir := filepath.Join(t.TempDir(), "skill")
	if _, err := Pull(client, skill, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	return dir
}

func writeLocal(t *testing.T, dir string, relative string, content string) {
	t.Helper()

	target := filepath.Join(dir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPullWritesFilesAndManifest(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/assets/logo.png", "\x89PNG\x00binary")

	dir := pullTestSkill(t, fake)

	skillContent, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil || string(skillContent) != "# Release Notes\n" {
		t.Fatalf("SKILL.md = %q, %v", skillContent, err)
	}

	logo, err := os.ReadFile(filepath.Join(dir, "assets", "logo.png"))
	if err != nil || string(logo) != "\x89PNG\x00binary" {
		t.Fatalf("logo.png = %q, %v", logo, err)
	}

	manifest, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if manifest.SkillID != skillstest.SkillID || len(manifest.Files) != 2 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestPushRoundTrip(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/references/old.md", "old")

	dir := pullTestSkill(t, fake)
	client := fake.Client(t)

	writeLocal(t, dir, "SKILL.md", "# Release Notes\n\nNew section.\n")
	writeLocal(t, dir, "scripts/build.sh", "echo build\n")
	writeLocal(t, dir, "assets/icon.png", "\x00\x01binary")
	if err := os.Remove(filepath.Join(dir, "references", "old.md")); err != nil {
		t.Fatal(err)
	}

	result, err := Push(client, dir, PushOptions{})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if len(result.Changes) != 4 {
		t.Fatalf("changes = %+v", result.Changes)
	}

	if content, _ := fake.Content("/SKILL.md"); content != "# Release Notes\n\nNew section." {
		t.Fatalf("remote SKILL.md = %q", content)
	}
	if content, _ := fake.Content("/assets/icon.png"); content != "\x00\x01binary" {
		t.Fatalf("remote icon.png = %q", content)
	}
	if fake.Uploads != 1 || len(fake.StorageErrors) != 0 {
		t.Fatalf("uploads = %d, storage errors = %v; want icon.png uploaded through a signed URL", fake.Uploads, fake.StorageErrors)
	}
	if _, exists := fake.Content("/references/old.md"); exists {
		t.Fatal("old.md should be deleted remotely")
	}

	status, err := GetStatus(client, dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if !status.Clean() {
		t.Fatalf("status after push = %+v", status)
	}
}

func TestPushRejectsRemoteChanges(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	client := fake.Client(t)

	fake.SetFile("/SKILL.md", "# Edited in the dashboard\n")
	writeLocal(t, dir, "SKILL.md", "# Edited locally\n")

	if _, err := Push(client, dir, PushOptions{}); !errors.Is(err, ErrRemoteChanged) {
		t.Fatalf("Push() error = %v, want ErrRemoteChanged", err)
	}
	if content, _ := fake.Content("/SKILL.md"); content != "# Edited in the dashboard" {
		t.Fatalf("remote SKILL.md was overwritten: %q", content)
	}
	if fake.Batches != 0 {
		t.Fatalf("the remote change should be caught before sending, got %d batch(es)", fake.Batches)
	}

	if _, err := Push(client, dir, PushOptions{Force: true}); err != nil {
		t.Fatalf("Push(force) error = %v", err)
	}
	if content, _ := fake.Content("/SKILL.md"); content != "# Edited locally" {
		t.Fatalf("remote SKILL.md after force = %q", content)
	}
}

func TestStatusReportsLocalAndRemoteChanges(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)

	writeLocal(t, dir, "notes.txt", "local")
	fake.SetFile("/references/new.md", "remote")

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if len(status.Local) != 1 || status.Local[0] != (Change{Path: "/notes.txt", Kind: ChangeAdded}) {
		t.Fatalf("local = %+v", status.Local)
	}
	if len(status.Remote) != 1 || status.Remote[0] != (Change{Path: "/references/new.md", Kind: ChangeAdded}) {
		t.Fatalf("remote = %+v", status.Remote)
	}
}

func TestPullRefusesToOverwriteLocalChanges(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	client := fake.Client(t)
	writeLocal(t, dir, "SKILL.md", "unsaved work")

	skill, err := client.GetSkill(skillstest.SkillID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Pull(client, skill, dir, PullOptions{}); err == nil {
		t.Fatal("Pull() should refuse to overwrite local changes")
	}

	content, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(content) != "unsaved work" {
		t.Fatalf("local SKILL.md was overwritten: %q", content)
	}
}

func TestPushRejectsUnsupportedPaths(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	writeLocal(t, dir, "bad*name.md", "x")

	if _, err := Push(fake.Client(t), dir, PushOptions{}); err == nil {
		t.Fatal("Push() should reject unsupported path characters")
	}
	if fake.Batches != 0 {
		t.Fatalf("no batch should be sent, got %d", fake.Batches)
	}
}

func TestPullRestoresDocumentTrailingNewline(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n\nLast line.\n")

	dir := pullTestSkill(t, fake)

	content, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil || string(content) != "# Release Notes\n\nLast line.\n" {
		t.Fatalf("SKILL.md = %q, %v", content, err)
	}

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil || !status.Clean() {
		t.Fatalf("status after pull = %+v, %v", status, err)
	}
}

func TestStatusReportsConflicts(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	fake.SetFile("/SKILL.md", "# Remote edit\n")
	writeLocal(t, dir, "SKILL.md", "# Local edit\n")

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if !status.Conflicted() || status.SkillSlug != skillstest.SkillSlug {
		t.Fatalf("status = %+v", status)
	}
}

func TestStatusChangesOnDifferentPathsDoNotConflict(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	fake.SetFile("/references/new.md", "remote")
	writeLocal(t, dir, "SKILL.md", "# Local edit\n")

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.Conflicted() || len(status.Local) != 1 || len(status.Remote) != 1 {
		t.Fatalf("status = %+v", status)
	}

	if _, err := Push(fake.Client(t), dir, PushOptions{}); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
}

func TestPullPreservesFileModes(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/scripts/build.sh", "echo build\n")

	dir := pullTestSkill(t, fake)
	script := filepath.Join(dir, "scripts", "build.sh")

	info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("SKILL.md mode = %v, want 0644", info.Mode().Perm())
	}

	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	fake.SetFile("/scripts/build.sh", "echo rebuilt\n")

	client := fake.Client(t)
	skill, err := client.GetSkill(skillstest.SkillID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(client, skill, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	info, err = os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("build.sh mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestStatusAndPushThroughSymlinkedDir(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/references/api.md", "api")

	dir := pullTestSkill(t, fake)
	client := fake.Client(t)

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}

	status, err := GetStatus(client, link)
	if err != nil || !status.Clean() {
		t.Fatalf("status through symlink = %+v, %v", status, err)
	}

	writeLocal(t, dir, "SKILL.md", "# Release Notes\n\nEdited.\n")
	result, err := Push(client, link, PushOptions{})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if len(result.Changes) != 1 || result.Changes[0] != (Change{Path: "/SKILL.md", Kind: ChangeModified}) {
		t.Fatalf("changes = %+v", result.Changes)
	}
	if _, exists := fake.Content("/references/api.md"); !exists {
		t.Fatal("api.md must not be deleted remotely")
	}
}

func TestStatusFollowsSymlinkedFiles(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/references/api.md", "api")

	dir := pullTestSkill(t, fake)

	target := filepath.Join(t.TempDir(), "api.md")
	if err := os.WriteFile(target, []byte("api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(dir, "references", "api.md")
	if err := os.Remove(apiPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, apiPath); err != nil {
		t.Fatal(err)
	}

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil || !status.Clean() {
		t.Fatalf("status with symlinked file = %+v, %v", status, err)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := GetStatus(fake.Client(t), dir); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("broken symlink error = %v", err)
	}
}

func TestPushRefusesToDeleteSkillFile(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/references/api.md", "api")

	dir := pullTestSkill(t, fake)
	for _, name := range []string{"SKILL.md", "references/api.md"} {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Push(fake.Client(t), dir, PushOptions{Force: true}); err == nil || !strings.Contains(err.Error(), "/SKILL.md") {
		t.Fatalf("Push() error = %v, want a /SKILL.md error", err)
	}
	if fake.Batches != 0 {
		t.Fatalf("no batch should be sent, got %d", fake.Batches)
	}
}

func TestPushChecksRemoteBeforeSendingBatch(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	writeLocal(t, dir, "notes.md", "local")
	fake.SetFile("/notes.md", "remote")

	if _, err := Push(fake.Client(t), dir, PushOptions{}); !errors.Is(err, ErrRemoteChanged) {
		t.Fatalf("Push() error = %v, want ErrRemoteChanged", err)
	}
	if fake.Batches != 0 {
		t.Fatalf("no batch should be sent, got %d", fake.Batches)
	}
}

func TestPushRecordsPartiallyAppliedBatch(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	client := fake.Client(t)

	writeLocal(t, dir, "a.md", "a\n")
	writeLocal(t, dir, "b.md", "b\n")
	writeLocal(t, dir, "c.txt", "c\n")
	fake.FailPath = "/b.md"

	_, err := Push(client, dir, PushOptions{})
	if err == nil || !strings.Contains(err.Error(), "1 of 3") {
		t.Fatalf("Push() error = %v, want a partial push error", err)
	}

	status, err := GetStatus(client, dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	want := []Change{{Path: "/b.md", Kind: ChangeAdded}, {Path: "/c.txt", Kind: ChangeAdded}}
	if len(status.Local) != 2 || status.Local[0] != want[0] || status.Local[1] != want[1] || len(status.Remote) != 0 {
		t.Fatalf("status after partial push = %+v", status)
	}

	fake.FailPath = ""
	if _, err := Push(client, dir, PushOptions{}); err != nil {
		t.Fatalf("Push() retry error = %v", err)
	}
	if status, err := GetStatus(client, dir); err != nil || !status.Clean() {
		t.Fatalf("status after retry = %+v, %v", status, err)
	}
}

func TestPullSkipsIgnoredRemotePaths(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/.git/config", "[core]")
	fake.SetFile("/.env", "TOKEN=secret")
	fake.SetFile("/docs/.DS_Store", "junk")

	client := fake.Client(t)
	skill, err := client.GetSkill(skillstest.SkillID)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "skill")
	result, err := Pull(client, skill, dir, PullOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if len(result.Written) != 1 || len(result.Skipped) != 3 {
		t.Fatalf("pull result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".env should not be written, stat error = %v", err)
	}

	status, err := GetStatus(client, dir)
	if err != nil || !status.Clean() {
		t.Fatalf("status after pull = %+v, %v", status, err)
	}
	if pushed, err := Push(client, dir, PushOptions{}); err != nil || len(pushed.Changes) != 0 {
		t.Fatalf("Push() = %+v, %v", pushed, err)
	}
	if _, exists := fake.Content("/.env"); !exists {
		t.Fatal(".env must not be deleted remotely")
	}
}

func TestLocalChangesSkipIgnoredFiles(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	for _, name := range []string{
		".env", ".env.local", "SKILL.md.swp", "SKILL.md~", ".metorial-123", ".vscode/settings.json",
		"node_modules/pkg/index.js", "drafts/idea.md", "references/scratch.tmp",
	} {
		writeLocal(t, dir, name, "ignored")
	}
	writeLocal(t, dir, ".metorialignore", "# local only\ndrafts/\nreferences/*.tmp\n")
	writeLocal(t, dir, "references/api.md", "api")

	manifest, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := localChanges(dir, manifest)
	if err != nil {
		t.Fatalf("localChanges() error = %v", err)
	}

	want := []Change{{Path: "/.metorialignore", Kind: ChangeAdded}, {Path: "/references/api.md", Kind: ChangeAdded}}
	if len(changes) != 2 || changes[0] != want[0] || changes[1] != want[1] {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestPullRejectsUnsafeAndCollidingPaths(t *testing.T) {
	for name, paths := range map[string][]string{
		"traversal": {"/../evil.md"},
		"backslash": {`/a\b.md`},
		"colon":     {"/a:b.md"},
		"case":      {"/Notes.md", "/notes.md"},
		"dir case":  {"/Docs/a.md", "/docs/b.md"},
	} {
		t.Run(name, func(t *testing.T) {
			fake := skillstest.NewServer(t)
			fake.SetFile("/SKILL.md", "# Release Notes\n")
			for _, filePath := range paths {
				fake.SetFile(filePath, "x")
			}

			client := fake.Client(t)
			skill, err := client.GetSkill(skillstest.SkillID)
			if err != nil {
				t.Fatal(err)
			}

			parent := t.TempDir()
			dir := filepath.Join(parent, "skill")
			if _, err := Pull(client, skill, dir, PullOptions{}); err == nil {
				t.Fatal("Pull() should fail")
			}
			if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("nothing should be written, stat error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(parent, "evil.md")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("evil.md escaped the skill folder, stat error = %v", err)
			}
		})
	}
}

func repull(t *testing.T, fake *skillstest.Server, dir string, options PullOptions) (*PullResult, error) {
	t.Helper()

	client := fake.Client(t)
	skill, err := client.GetSkill(skillstest.SkillID)
	if err != nil {
		t.Fatal(err)
	}

	return Pull(client, skill, dir, options)
}

func assertClean(t *testing.T, fake *skillstest.Server, dir string) {
	t.Helper()

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil || !status.Clean() {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

func TestPullAppliesRemoteCaseOnlyRename(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/Notes.md", "notes\n")

	dir := pullTestSkill(t, fake)
	fake.DeleteFile("/Notes.md")
	fake.SetFile("/notes.md", "notes\n")

	result, err := repull(t, fake, dir, PullOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if len(result.Removed) != 1 || result.Removed[0] != "/Notes.md" {
		t.Fatalf("removed = %v", result.Removed)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[entry.Name()] = true
	}
	if !names["notes.md"] || names["Notes.md"] {
		t.Fatalf("directory entries = %v, want notes.md only", names)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "notes.md")); err != nil || string(content) != "notes\n" {
		t.Fatalf("notes.md = %q, %v", content, err)
	}

	assertClean(t, fake, dir)
	if pushed, err := Push(fake.Client(t), dir, PushOptions{}); err != nil || len(pushed.Changes) != 0 {
		t.Fatalf("Push() = %+v, %v", pushed, err)
	}
	if _, exists := fake.Content("/notes.md"); !exists {
		t.Fatal("notes.md must not be deleted remotely")
	}
}

func TestPullReplacesFileWithFolderAndBack(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/docs", "plain file")

	dir := pullTestSkill(t, fake)
	fake.DeleteFile("/docs")
	fake.SetFile("/docs/a.md", "a\n")

	if _, err := repull(t, fake, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() file to folder error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "docs", "a.md")); err != nil || string(content) != "a\n" {
		t.Fatalf("docs/a.md = %q, %v", content, err)
	}
	assertClean(t, fake, dir)

	fake.DeleteFile("/docs/a.md")
	fake.SetFile("/docs", "file again")

	if _, err := repull(t, fake, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() folder to file error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "docs")); err != nil || string(content) != "file again" {
		t.Fatalf("docs = %q, %v", content, err)
	}
	assertClean(t, fake, dir)
}

func TestPullFailingPartwayRecordsWrittenFiles(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/a.md", "a\n")
	fake.SetFile("/b.md", "b\n")
	fake.FailReadPath = "/b.md"

	dir := filepath.Join(t.TempDir(), "skill")
	if _, err := repull(t, fake, dir, PullOptions{}); err == nil {
		t.Fatal("Pull() should fail")
	}

	manifest, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if _, ok := manifest.Files["/a.md"]; !ok || len(manifest.Files) != 2 {
		t.Fatalf("manifest files = %+v, want /SKILL.md and /a.md", manifest.Files)
	}

	fake.FailReadPath = ""
	if _, err := repull(t, fake, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() retry error = %v", err)
	}
	assertClean(t, fake, dir)
}

func TestPullRefusesToReplaceSymlinkedFile(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/references/api.md", "api")

	dir := pullTestSkill(t, fake)

	target := filepath.Join(t.TempDir(), "api.md")
	if err := os.WriteFile(target, []byte("api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(dir, "references", "api.md")
	if err := os.Remove(apiPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, apiPath); err != nil {
		t.Fatal(err)
	}

	if _, err := repull(t, fake, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() with an unchanged symlinked file error = %v", err)
	}

	fake.SetFile("/references/api.md", "api v2")
	if _, err := repull(t, fake, dir, PullOptions{}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Pull() error = %v, want a symbolic link error", err)
	}

	if info, err := os.Lstat(apiPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("api.md should still be a symlink, info = %v, err = %v", info, err)
	}
	if content, _ := os.ReadFile(target); string(content) != "api\n" {
		t.Fatalf("link target = %q, want it untouched", content)
	}
}

func TestIgnoreFileWithTooManyPatternsIsAnError(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	patterns := []string{}
	for index := range 31 {
		patterns = append(patterns, fmt.Sprintf("draft-%d", index))
	}
	writeLocal(t, dir, ".metorialignore", strings.Join(patterns, "\n")+"\n")

	want := "has 31 patterns; the limit is 30"
	if _, err := GetStatus(fake.Client(t), dir); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if _, err := Push(fake.Client(t), dir, PushOptions{}); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Push() error = %v", err)
	}
	if _, err := repull(t, fake, dir, PullOptions{Force: true}); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Pull() error = %v", err)
	}
}

func TestPullAllowsExistingIgnoreFile(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := filepath.Join(t.TempDir(), "skill")
	writeLocal(t, dir, ".metorialignore", "drafts/\n")

	if _, err := repull(t, fake, dir, PullOptions{}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	status, err := GetStatus(fake.Client(t), dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.Local) != 1 || status.Local[0] != (Change{Path: "/.metorialignore", Kind: ChangeAdded}) {
		t.Fatalf("local = %+v, want .metorialignore to be pushed like any other file", status.Local)
	}

	other := skillstest.NewServer(t)
	other.SetFile("/SKILL.md", "# Release Notes\n")
	other.SetFile("/.metorialignore", "scratch/\n")

	dir = filepath.Join(t.TempDir(), "skill")
	writeLocal(t, dir, ".metorialignore", "drafts/\n")
	if _, err := repull(t, other, dir, PullOptions{}); err == nil || !strings.Contains(err.Error(), ".metorialignore") {
		t.Fatalf("Pull() error = %v, want the remote .metorialignore to be protected", err)
	}
	if content, _ := os.ReadFile(filepath.Join(dir, ".metorialignore")); string(content) != "drafts/\n" {
		t.Fatalf("local .metorialignore = %q, want it untouched", content)
	}
}

func TestPushRejectsInvalidUTF8Markdown(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	writeLocal(t, dir, "notes.md", "caf\xe9\n")

	if _, err := Push(fake.Client(t), dir, PushOptions{}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("Push() error = %v, want a UTF-8 error", err)
	}
	if fake.Batches != 0 {
		t.Fatalf("no batch should be sent, got %d", fake.Batches)
	}
}

func TestPushRecordsPartiallyAppliedDocumentWithoutTrailingNewline(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	client := fake.Client(t)

	writeLocal(t, dir, "a.md", "a")
	writeLocal(t, dir, "b.md", "b\n")
	writeLocal(t, dir, "c.txt", "c\n")
	fake.FailPath = "/b.md"

	if _, err := Push(client, dir, PushOptions{}); err == nil || !strings.Contains(err.Error(), "1 of 3") {
		t.Fatalf("Push() error = %v, want a partial push error", err)
	}

	status, err := GetStatus(client, dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	want := []Change{{Path: "/b.md", Kind: ChangeAdded}, {Path: "/c.txt", Kind: ChangeAdded}}
	if len(status.Local) != 2 || status.Local[0] != want[0] || status.Local[1] != want[1] || len(status.Remote) != 0 {
		t.Fatalf("status after partial push = %+v", status)
	}
}

func TestPushReportsEarlierBatchesWhenLaterBatchFails(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	for index := range maxBatchChanges {
		writeLocal(t, dir, fmt.Sprintf("references/f%03d.md", index), "x\n")
	}
	writeLocal(t, dir, "z.bin", "\x00binary")
	fake.FailUploads = true

	_, err := Push(fake.Client(t), dir, PushOptions{})
	if err == nil || !strings.Contains(err.Error(), "100 of 101") {
		t.Fatalf("Push() error = %v, want the earlier batch to be reported", err)
	}
	if fake.Batches != 1 {
		t.Fatalf("batches = %d, want 1", fake.Batches)
	}

	status, err := GetStatus(fake.Client(t), dir)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.Local) != 1 || status.Local[0] != (Change{Path: "/z.bin", Kind: ChangeAdded}) {
		t.Fatalf("local = %+v", status.Local)
	}
}

func TestIgnorePatternsMatchFoldersAndAnchors(t *testing.T) {
	dir := t.TempDir()
	writeLocal(t, dir, ".metorialignore", "docs/old\n/drafts/\nscratch\nreferences/*\n/\n")

	rules, err := loadIgnoreRules(dir)
	if err != nil {
		t.Fatalf("loadIgnoreRules() error = %v", err)
	}

	for remotePath, want := range map[string]bool{
		"/docs/old":               true,
		"/docs/old/a.md":          true,
		"/docs/old/deep/b.md":     true,
		"/docs/older/a.md":        false,
		"/notes/docs/old/a.md":    false,
		"/drafts":                 true,
		"/drafts/idea.md":         true,
		"/notes/drafts/idea.md":   false,
		"/scratch/a.md":           true,
		"/notes/scratch":          true,
		"/references/api.md":      true,
		"/references/sub/deep.md": true,
		"/references.md":          false,
		"/SKILL.md":               false,
	} {
		if got := rules.matches(remotePath); got != want {
			t.Errorf("matches(%q) = %v, want %v", remotePath, got, want)
		}
	}
}

func TestIgnoredFolderStaysOutOfPullStatusAndPush(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/.metorialignore", "docs/old\n")
	fake.SetFile("/docs/old/a.md", "old\n")
	fake.SetFile("/docs/keep.md", "keep\n")

	dir := filepath.Join(t.TempDir(), "skill")
	writeLocal(t, dir, ".metorialignore", "docs/old\n")

	result, err := repull(t, fake, dir, PullOptions{Force: true})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "/docs/old/a.md" {
		t.Fatalf("skipped = %v, want /docs/old/a.md", result.Skipped)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "old", "a.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("docs/old/a.md should not be written, stat error = %v", err)
	}
	assertClean(t, fake, dir)

	writeLocal(t, dir, "docs/old/local.md", "local only\n")
	fake.SetFile("/docs/old/a.md", "old v2\n")
	assertClean(t, fake, dir)

	if pushed, err := Push(fake.Client(t), dir, PushOptions{}); err != nil || len(pushed.Changes) != 0 {
		t.Fatalf("Push() = %+v, %v", pushed, err)
	}
	if _, exists := fake.Content("/docs/old/a.md"); !exists {
		t.Fatal("docs/old/a.md must not be deleted remotely")
	}
	if fake.Batches != 0 {
		t.Fatalf("batches = %d, want 0", fake.Batches)
	}
}

func TestPullIntoCurrentDirectoryReplacesFolderWithFile(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")
	fake.SetFile("/docs/a.md", "a\n")

	t.Chdir(t.TempDir())
	if _, err := repull(t, fake, ".", PullOptions{}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	fake.DeleteFile("/docs/a.md")
	fake.SetFile("/docs", "file now")

	if _, err := repull(t, fake, ".", PullOptions{}); err != nil {
		t.Fatalf("Pull() folder to file error = %v", err)
	}
	if content, err := os.ReadFile("docs"); err != nil || string(content) != "file now" {
		t.Fatalf("docs = %q, %v", content, err)
	}
	assertClean(t, fake, ".")
}

func TestPushValidatesEveryBatchBeforeSending(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.SetFile("/SKILL.md", "# Release Notes\n")

	dir := pullTestSkill(t, fake)
	for index := range maxBatchChanges {
		writeLocal(t, dir, fmt.Sprintf("references/f%03d.md", index), "x\n")
	}
	writeLocal(t, dir, "z.md", "caf\xe9\n")

	if _, err := Push(fake.Client(t), dir, PushOptions{}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("Push() error = %v, want a UTF-8 error", err)
	}
	if fake.Batches != 0 {
		t.Fatalf("batches = %d, want none before the invalid file is fixed", fake.Batches)
	}
}
