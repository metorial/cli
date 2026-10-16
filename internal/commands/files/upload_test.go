package files

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/metorial/cli/internal/config"
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
	if _, err := os.Stat(source.Path); err != nil {
		t.Fatalf("expected archive to exist: %v", err)
	}

	cleanup()
	if _, err := os.Stat(source.Path); !os.IsNotExist(err) {
		t.Fatalf("expected archive to be removed, got %v", err)
	}
}

func TestPrepareSourceRejectsDirectoriesWithoutFiles(t *testing.T) {
	_, _, err := prepareSource(t.TempDir(), "", io.Discard)
	if err == nil {
		t.Fatal("expected an error for an empty directory")
	}
}

func TestUploadSendsSmallFilesAsMultipart(t *testing.T) {
	local := filepath.Join(t.TempDir(), "notes.txt")
	writeFile(t, local, "hello")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/files" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}

		expected := map[string]string{
			"purpose":       "generic",
			"title":         "Notes",
			"instance_id":   "inst_1",
			"store_id":      "store_1",
			"path":          "docs/notes.txt",
			"store_replace": "true",
		}
		for key, value := range expected {
			if got := r.FormValue(key); got != value {
				t.Errorf("expected %s=%q, got %q", key, value, got)
			}
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(file)
		if header.Filename != "notes.txt" || string(data) != "hello" {
			t.Errorf("unexpected file %q with contents %q", header.Filename, data)
		}

		writeJSON(w, http.StatusOK, map[string]any{"object": "file", "id": "file_1"})
	}))
	defer server.Close()

	response, err := newTestUploader(t, server.URL, 1024).upload(context.Background(), localSource(t, local), uploadOptions{
		Purpose:      "generic",
		Title:        "Notes",
		InstanceID:   "inst_1",
		StoreID:      "store_1",
		StorePath:    "docs/notes.txt",
		StoreReplace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFileID(t, response.Body, "file_1")
}

func TestUploadSendsLargeFilesThroughAnUploadURL(t *testing.T) {
	local := filepath.Join(t.TempDir(), "archive.bin")
	writeFile(t, local, "large contents")

	steps := []string{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/storage":
			steps = append(steps, "put")
			if r.Header.Get("Authorization") != "" {
				t.Error("expected no Metorial credentials on the storage request")
			}
			if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("expected the declared content type, got %q", got)
			}
			data, _ := io.ReadAll(r.Body)
			if string(data) != "large contents" {
				t.Errorf("unexpected uploaded contents %q", data)
			}
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost && r.URL.Path == "/files":
			body := map[string]any{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}

			switch body["mode"] {
			case "get_upload_url":
				steps = append(steps, "get_upload_url")
				if body["file_name"] != "archive.bin" || body["file_size"] != float64(len("large contents")) || body["purpose"] != "skill_export" {
					t.Errorf("unexpected upload URL request %v", body)
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"object": "file_upload",
					"id":     "fup_1",
					"upload": map[string]any{"url": server.URL + "/storage", "method": "PUT"},
				})
			case "complete":
				steps = append(steps, "complete")
				if body["file_upload_id"] != "fup_1" {
					t.Errorf("unexpected complete request %v", body)
				}
				writeJSON(w, http.StatusOK, map[string]any{"object": "file", "id": "file_2"})
			default:
				t.Errorf("unexpected mode %v", body["mode"])
			}

		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	response, err := newTestUploader(t, server.URL, 4).upload(context.Background(), localSource(t, local), uploadOptions{Purpose: "skill_export"})
	if err != nil {
		t.Fatal(err)
	}
	assertFileID(t, response.Body, "file_2")

	if expected := []string{"get_upload_url", "put", "complete"}; !slices.Equal(steps, expected) {
		t.Fatalf("expected steps %v, got %v", expected, steps)
	}
}

func TestUploadReturnsAPIErrorResponses(t *testing.T) {
	local := filepath.Join(t.TempDir(), "notes.txt")
	writeFile(t, local, "hello")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": "Unknown file purpose"})
	}))
	defer server.Close()

	response, err := newTestUploader(t, server.URL, 1024).upload(context.Background(), localSource(t, local), uploadOptions{Purpose: "nope"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if response == nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected the error response to be returned, got %+v", response)
	}
}

func TestUploadReportsStorageFailures(t *testing.T) {
	local := filepath.Join(t.TempDir(), "archive.bin")
	writeFile(t, local, "large contents")

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			http.Error(w, "SignatureDoesNotMatch", http.StatusForbidden)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "fup_1",
			"upload": map[string]any{"url": server.URL + "/storage", "method": "PUT"},
		})
	}))
	defer server.Close()

	_, err := newTestUploader(t, server.URL, 4).upload(context.Background(), localSource(t, local), uploadOptions{Purpose: "generic"})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("SignatureDoesNotMatch")) {
		t.Fatalf("expected the storage error to be reported, got %v", err)
	}
}

func newTestUploader(t *testing.T, host string, directLimit int64) *uploader {
	t.Helper()

	hostURL, err := url.Parse(host)
	if err != nil {
		t.Fatal(err)
	}
	uploader := newUploader(config.Runtime{APIKey: "metorial_sk_test", APIHost: host, APIHostURL: hostURL})
	uploader.directLimit = directLimit
	return uploader
}

func localSource(t *testing.T, path string) uploadSource {
	t.Helper()

	source, _, err := prepareSource(path, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return source
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func assertFileID(t *testing.T, body []byte, expected string) {
	t.Helper()

	file := map[string]any{}
	if err := json.Unmarshal(body, &file); err != nil {
		t.Fatal(err)
	}
	if file["id"] != expected {
		t.Fatalf("expected file %s, got %v", expected, file["id"])
	}
}
