package sandbox

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/sandboxaccess"
)

type downloadFixture struct {
	transfer *transfer
	notices  *bytes.Buffer
	root     *os.Root
	target   string
	outside  string
}

func newDownloadFixture(t *testing.T) *downloadFixture {
	t.Helper()

	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	target := filepath.Join(parent, "download")
	outside := filepath.Join(parent, "outside")

	for _, directory := range []string{target, outside} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("original"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	root, err := os.OpenRoot(target)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	notices := &bytes.Buffer{}
	return &downloadFixture{
		transfer: &transfer{stderr: notices, progress: &progress{writer: io.Discard}},
		notices:  notices,
		root:     root,
		target:   target,
		outside:  outside,
	}
}

func (f *downloadFixture) symlink(t *testing.T, target, name string) {
	t.Helper()

	if err := os.Symlink(target, filepath.Join(f.target, filepath.FromSlash(name))); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

func (f *downloadFixture) expectOutsideUntouched(t *testing.T) {
	t.Helper()

	entries, err := os.ReadDir(f.outside)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one entry outside the download directory, got %d, %v", len(entries), err)
	}
	content, err := os.ReadFile(filepath.Join(f.outside, "secret.txt"))
	if err != nil || string(content) != "original" {
		t.Fatalf("expected the file outside the download directory to be untouched, got %q, %v", content, err)
	}
}

func (f *downloadFixture) expectMissing(t *testing.T, name string) {
	t.Helper()

	if _, err := os.Lstat(filepath.Join(f.target, filepath.FromSlash(name))); !os.IsNotExist(err) {
		t.Fatalf("expected %s not to exist, got %v", name, err)
	}
}

func paths(entries []sandboxaccess.FileEntry) []string {
	result := []string{}
	for _, entry := range entries {
		result = append(result, entry.Path)
	}
	return result
}

func TestLinkTargetStaysInside(t *testing.T) {
	cases := []struct {
		link   string
		target string
		want   bool
	}{
		{"link", "file.txt", true},
		{"link", "docs/guide.md", true},
		{"link", "./docs/", true},
		{"link", ".", true},
		{"docs/link", "guide.md", true},
		{"docs/link", "../README.md", true},
		{"docs/link", "..", true},
		{"a/b/link", "../../c/d", true},

		{"link", "", false},
		{"link", "/etc/passwd", false},
		{"link", "..", false},
		{"link", "../outside", false},
		{"docs/link", "../..", false},
		{"docs/link", "../../outside", false},
		{"a/b/link", "../../../outside", false},
		{"link", "docs/../..", false},
		{"link", "docs/../../outside", false},
		{"docs/link", "sub/../../../outside", false},
		{"link", "docs/..", false},
	}

	for _, tc := range cases {
		link := filepath.FromSlash(tc.link)
		if got := linkTargetStaysInside(link, tc.target); got != tc.want {
			t.Errorf("linkTargetStaysInside(%q, %q) = %t, want %t", tc.link, tc.target, got, tc.want)
		}
	}
}

func TestPrepareTreeRestoresSymlinksInsideTheTree(t *testing.T) {
	f := newDownloadFixture(t)

	files, total, err := f.transfer.prepareTree(f.root, f.target, []sandboxaccess.FileEntry{
		{Path: "docs", Type: "directory"},
		{Path: "docs/guide.md", Type: "file", Size: 5},
		{Path: "latest", Type: "symlink", Target: "docs"},
		{Path: "docs/readme", Type: "symlink", Target: "../README.md"},
		{Path: "docs/alias.md", Type: "symlink", Target: "guide.md"},
		{Path: "README.md", Type: "file", Size: 7},
	})
	if err != nil {
		t.Fatalf("prepareTree: %v", err)
	}
	if strings.Join(paths(files), ",") != "docs/guide.md,README.md" || total != 12 {
		t.Fatalf("unexpected files %v (%d bytes)", paths(files), total)
	}

	for name, want := range map[string]string{
		"latest":        "docs",
		"docs/readme":   filepath.FromSlash("../README.md"),
		"docs/alias.md": "guide.md",
	} {
		got, err := os.Readlink(filepath.Join(f.target, filepath.FromSlash(name)))
		if err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		if got != want {
			t.Fatalf("symlink %s points to %q, want %q", name, got, want)
		}
	}
	if f.notices.Len() != 0 {
		t.Fatalf("unexpected notices: %s", f.notices)
	}
}

func TestPrepareTreeSkipsSymlinksThatLeaveTheTree(t *testing.T) {
	f := newDownloadFixture(t)

	entries := []sandboxaccess.FileEntry{{Path: "docs", Type: "directory"}}
	targets := map[string]string{
		"absolute":      f.outside,
		"parent":        "..",
		"sibling":       "../outside",
		"docs/deep":     "../../outside/secret.txt",
		"through-child": "docs/../../outside",
	}
	for name, target := range targets {
		entries = append(entries, sandboxaccess.FileEntry{Path: name, Type: "symlink", Target: target})
	}

	if _, _, err := f.transfer.prepareTree(f.root, f.target, entries); err != nil {
		t.Fatalf("prepareTree: %v", err)
	}

	for name := range targets {
		f.expectMissing(t, name)
		if !strings.Contains(f.notices.String(), "skipping symlink "+name+":") {
			t.Fatalf("expected a notice for %s, got: %s", name, f.notices)
		}
	}
}

func TestPrepareTreeNeverCreatesEntriesThroughAnExistingSymlink(t *testing.T) {
	f := newDownloadFixture(t)
	if err := os.Mkdir(filepath.Join(f.target, "real"), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	f.symlink(t, f.outside, "escape")
	f.symlink(t, "real", "alias")

	files, _, err := f.transfer.prepareTree(f.root, f.target, []sandboxaccess.FileEntry{
		{Path: "escape", Type: "directory"},
		{Path: "escape/nested", Type: "directory"},
		{Path: "escape/secret.txt", Type: "file", Size: 3},
		{Path: "escape/nested/new.txt", Type: "file", Size: 3},
		{Path: "escape/link", Type: "symlink", Target: "secret.txt"},
		{Path: "alias/inside.txt", Type: "file", Size: 3},
		{Path: "alias/sub", Type: "directory"},
		{Path: "real/kept.txt", Type: "file", Size: 3},
	})
	if err != nil {
		t.Fatalf("prepareTree: %v", err)
	}

	if strings.Join(paths(files), ",") != "real/kept.txt" {
		t.Fatalf("unexpected files %v", paths(files))
	}
	f.expectOutsideUntouched(t)
	f.expectMissing(t, "real/sub")

	notices := f.notices.String()
	if strings.Count(notices, "beneath escape:") != 1 || strings.Count(notices, "beneath alias:") != 1 {
		t.Fatalf("expected one notice per symbolic link, got: %s", notices)
	}
}

func TestPrepareTreeReplacesAStaleSymlink(t *testing.T) {
	f := newDownloadFixture(t)
	f.symlink(t, f.outside, "latest")

	_, _, err := f.transfer.prepareTree(f.root, f.target, []sandboxaccess.FileEntry{
		{Path: "docs", Type: "directory"},
		{Path: "latest", Type: "symlink", Target: "docs"},
	})
	if err != nil {
		t.Fatalf("prepareTree: %v", err)
	}

	got, err := os.Readlink(filepath.Join(f.target, "latest"))
	if err != nil || got != "docs" {
		t.Fatalf("expected the symlink to be replaced, got %q, %v", got, err)
	}
}

func TestPrepareTreeRemovesSymlinksThatResolveOutsideTheTree(t *testing.T) {
	f := newDownloadFixture(t)
	f.symlink(t, f.outside, "stale")

	_, _, err := f.transfer.prepareTree(f.root, f.target, []sandboxaccess.FileEntry{
		{Path: "shortcut", Type: "symlink", Target: "stale/secret.txt"},
		{Path: "dangling", Type: "symlink", Target: "missing/file.txt"},
	})
	if err != nil {
		t.Fatalf("prepareTree: %v", err)
	}

	f.expectMissing(t, "shortcut")
	if _, err := os.Lstat(filepath.Join(f.target, "dangling")); err != nil {
		t.Fatalf("expected the dangling symlink to be kept: %v", err)
	}
	f.expectOutsideUntouched(t)
}

func TestPrepareTreeSkipsUnsafePaths(t *testing.T) {
	f := newDownloadFixture(t)

	files, _, err := f.transfer.prepareTree(f.root, f.target, []sandboxaccess.FileEntry{
		{Path: "../outside/new.txt", Type: "file"},
		{Path: "/etc/new.txt", Type: "file"},
		{Path: "../outside/dir", Type: "directory"},
		{Path: "../outside/link", Type: "symlink", Target: "secret.txt"},
	})
	if err != nil {
		t.Fatalf("prepareTree: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("unexpected files %v", paths(files))
	}
	f.expectOutsideUntouched(t)
}

func fill(content string) func(io.Writer) error {
	return func(writer io.Writer) error {
		_, err := writer.Write([]byte(content))
		return err
	}
}

func TestWriteDownloadCreatesFilesAndParents(t *testing.T) {
	f := newDownloadFixture(t)
	name := filepath.Join("docs", "deep", "guide.md")

	if err := writeDownload(f.root, name, sandboxaccess.FileEntry{Mode: 0o640}, fill("guide")); err != nil {
		t.Fatalf("writeDownload: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(f.target, name))
	if err != nil || string(content) != "guide" {
		t.Fatalf("unexpected content %q, %v", content, err)
	}
	entries, err := os.ReadDir(filepath.Join(f.target, "docs", "deep"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected only the downloaded file, got %d entries, %v", len(entries), err)
	}
}

func TestWriteDownloadReplacesASymlinkInsteadOfWritingThroughIt(t *testing.T) {
	f := newDownloadFixture(t)
	f.symlink(t, filepath.Join(f.outside, "secret.txt"), "notes.txt")

	if err := writeDownload(f.root, "notes.txt", sandboxaccess.FileEntry{}, fill("downloaded")); err != nil {
		t.Fatalf("writeDownload: %v", err)
	}

	info, err := os.Lstat(filepath.Join(f.target, "notes.txt"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("expected a regular file, got %v, %v", info, err)
	}
	f.expectOutsideUntouched(t)
}

func TestWriteDownloadRefusesASymlinkedParent(t *testing.T) {
	f := newDownloadFixture(t)
	f.symlink(t, f.outside, "escape")

	err := writeDownload(f.root, filepath.Join("escape", "new.txt"), sandboxaccess.FileEntry{}, fill("downloaded"))

	var throughLink *symlinkInPathError
	if !errors.As(err, &throughLink) {
		t.Fatalf("expected the symbolic link to be refused, got %v", err)
	}
	f.expectOutsideUntouched(t)
}

func TestWriteDownloadRemovesTheTemporaryFileOnFailure(t *testing.T) {
	f := newDownloadFixture(t)
	failure := errors.New("connection lost")

	err := writeDownload(f.root, "partial.bin", sandboxaccess.FileEntry{}, func(writer io.Writer) error {
		_, _ = writer.Write([]byte("partial"))
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("expected the read error, got %v", err)
	}

	entries, readErr := os.ReadDir(f.target)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("expected an empty directory, got %d entries, %v", len(entries), readErr)
	}
}
