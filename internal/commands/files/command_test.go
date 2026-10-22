package files

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestZipDirectoryStoresContentsRelativeToTheDirectory(t *testing.T) {
	directory := t.TempDir()
	writeFile(t, filepath.Join(directory, "Dockerfile"), "FROM alpine")
	writeFile(t, filepath.Join(directory, "src", "main.go"), "package main")
	if err := os.MkdirAll(filepath.Join(directory, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Dockerfile", filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	exclude := filepath.Join(directory, "archive.zip")
	writeFile(t, exclude, "partial")

	buffer := &bytes.Buffer{}
	result, err := zipDirectory(directory, buffer, exclude)
	if err != nil {
		t.Fatal(err)
	}

	if result.Files != 2 {
		t.Fatalf("expected 2 files, got %d", result.Files)
	}
	if !slices.Equal(result.Skipped, []string{"link"}) {
		t.Fatalf("expected only the symlink to be skipped, got %v", result.Skipped)
	}

	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	contents := map[string]string{}
	for _, file := range reader.File {
		names = append(names, file.Name)
		if file.FileInfo().IsDir() {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(handle)
		_ = handle.Close()
		contents[file.Name] = string(data)
	}

	slices.Sort(names)
	if expected := []string{"Dockerfile", "empty/", "src/", "src/main.go"}; !slices.Equal(names, expected) {
		t.Fatalf("expected entries %v, got %v", expected, names)
	}
	if contents["src/main.go"] != "package main" {
		t.Fatalf("unexpected nested file contents %q", contents["src/main.go"])
	}
}

func TestPrepareSourceZipsDirectories(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "context")
	writeFile(t, filepath.Join(directory, "Dockerfile"), "FROM alpine")

	stderr := &bytes.Buffer{}
	source, cleanup, err := prepareSource(directory, "", stderr)
	if err != nil {
		t.Fatal(err)
	}

	if source.Name != "context.zip" || source.ContentType != "application/zip" {
		t.Fatalf("unexpected source %+v", source)
	}
	archive, err := source.Open()
	if err != nil {
		t.Fatalf("expected archive to exist: %v", err)
	}
	_ = archive.Close()

	cleanup()
	if _, err := source.Open(); !os.IsNotExist(err) {
		t.Fatalf("expected archive to be removed, got %v", err)
	}
}

func TestPrepareSourceRejectsDirectoriesWithoutFiles(t *testing.T) {
	_, _, err := prepareSource(t.TempDir(), "", io.Discard)
	if err == nil {
		t.Fatal("expected an error for an empty directory")
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
