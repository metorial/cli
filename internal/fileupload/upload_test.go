package fileupload

import (
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
	"strings"
	"testing"

	"github.com/metorial/cli/internal/config"
)

const unavailableMessage = "Signed upload URLs are not available for the configured object storage"

func TestUploadSendsFilesThroughAnUploadURL(t *testing.T) {
	local := filepath.Join(t.TempDir(), "archive.bin")
	if err := os.WriteFile(local, []byte("file contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	steps := []string{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/storage":
			steps = append(steps, "put")
			if r.Header.Get("Authorization") != "" || r.Header.Get("Metorial-Version") != "" {
				t.Error("expected no Metorial headers on the storage request")
			}
			if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("expected the declared content type, got %q", got)
			}
			data, _ := io.ReadAll(r.Body)
			if string(data) != "file contents" {
				t.Errorf("unexpected uploaded contents %q", data)
			}
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost && r.URL.Path == "/files":
			if r.Header.Get("Metorial-Version") != "test_version" {
				t.Errorf("expected the client headers on the files request, got %v", r.Header)
			}
			body := map[string]any{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}

			switch body["mode"] {
			case "get_upload_url":
				steps = append(steps, "get_upload_url")
				expected := map[string]any{
					"purpose":       "skill_export",
					"file_name":     "archive.bin",
					"file_size":     float64(len("file contents")),
					"file_type":     "application/octet-stream",
					"title":         "Archive",
					"instance_id":   "inst_1",
					"store_id":      "store_1",
					"path":          "docs/archive.bin",
					"store_replace": true,
				}
				for key, value := range expected {
					if body[key] != value {
						t.Errorf("expected %s=%v, got %v", key, value, body[key])
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"object": "file_upload",
					"id":     "fup_1",
					"upload": map[string]any{"url": server.URL + "/storage", "method": "PUT"},
				})
			case "complete":
				steps = append(steps, "complete")
				if body["file_upload_id"] != "fup_1" || body["instance_id"] != "inst_1" {
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

	response, err := newTestUploader(t, server.URL).Upload(context.Background(), FileSource(local, "archive.bin", "application/octet-stream", int64(len("file contents"))), Options{
		Purpose:      "skill_export",
		Title:        "Archive",
		InstanceID:   "inst_1",
		StoreID:      "store_1",
		StorePath:    "docs/archive.bin",
		StoreReplace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFileID(t, response.Body, "file_2")

	if expected := []string{"get_upload_url", "put", "complete"}; !slices.Equal(steps, expected) {
		t.Fatalf("expected steps %v, got %v", expected, steps)
	}
}

func TestUploadReturnsAPIErrorResponses(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "bad_request", "message": unavailableMessage})
	}))
	defer server.Close()

	response, err := newTestUploader(t, server.URL).Upload(context.Background(), BytesSource("notes.txt", []byte("hello")), Options{Purpose: "generic"})
	if err == nil || !strings.Contains(err.Error(), unavailableMessage) {
		t.Fatalf("expected the API error, got %v", err)
	}
	if !strings.Contains(err.Error(), "configure UPLOAD_HOST or presign-capable object storage") {
		t.Fatalf("expected a storage configuration hint, got %v", err)
	}
	if response == nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected the error response to be returned, got %+v", response)
	}
	if requests != 1 {
		t.Fatalf("expected a single request, got %d", requests)
	}
}

func TestUploadReportsStorageFailures(t *testing.T) {
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

	_, err := newTestUploader(t, server.URL).Upload(context.Background(), BytesSource("archive.bin", []byte("large contents")), Options{Purpose: "generic"})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("SignatureDoesNotMatch")) {
		t.Fatalf("expected the storage error to be reported, got %v", err)
	}
}

func newTestUploader(t *testing.T, host string) *Uploader {
	t.Helper()

	hostURL, err := url.Parse(host)
	if err != nil {
		t.Fatal(err)
	}
	uploader := New(config.Runtime{APIKey: "metorial_sk_test", APIHost: host, APIHostURL: hostURL})
	uploader.Headers = map[string]string{"Metorial-Version": "test_version"}
	return uploader
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

func TestNewUploaderTimesOutStorageRequests(t *testing.T) {
	uploader := New(config.Runtime{})

	transport, ok := uploader.Client.Transport.(*http.Transport)
	if !ok || transport.ResponseHeaderTimeout <= 0 || uploader.Client.Timeout <= 0 {
		t.Fatalf("expected the storage client to time out, got client timeout %v and transport %#v", uploader.Client.Timeout, uploader.Client.Transport)
	}
}
