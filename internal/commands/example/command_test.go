package example

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
)

func TestExampleRecordsGenerateFriendlyIdentifiers(t *testing.T) {
	manifest := &exampleManifest{
		SDKs: []exampleSDK{
			{ID: "metorial-node", Name: "Metorial Node"},
		},
		Examples: []exampleManifestItem{
			{
				RepositoryURL: "https://github.com/metorial/metorial-node",
				Directory:     "examples/typescript-openai",
				Name:          "Metorial + OpenAI",
				Description:   "OpenAI example",
				SDK:           "metorial-node",
			},
		},
	}

	records := manifest.records()
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Identifier != "metorial-openai" {
		t.Fatalf("identifier = %q, want %q", records[0].Identifier, "metorial-openai")
	}
	if records[0].DefaultPath != "metorial_openai" {
		t.Fatalf("default path = %q, want %q", records[0].DefaultPath, "metorial_openai")
	}
}

func TestFindExampleRecordAcceptsRepositoryPathIdentifier(t *testing.T) {
	records := []exampleRecord{
		{
			Identifier:    "metorial-openai",
			Name:          "Metorial + OpenAI",
			RepositoryURL: "https://github.com/metorial/metorial-node",
			Directory:     "examples/typescript-openai",
		},
	}

	record, err := findExampleRecord(records, "metorial-node/examples/typescript-openai")
	if err != nil {
		t.Fatalf("findExampleRecord() error = %v", err)
	}
	if record.Identifier != "metorial-openai" {
		t.Fatalf("record.Identifier = %q, want %q", record.Identifier, "metorial-openai")
	}
}

func TestExampleListStructuredOutputIncludesIdentifier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cli/manifest.json":
			_, _ = writer.Write([]byte(`{"sdks":[{"id":"metorial-node","name":"Metorial Node"}],"examples":[{"repositoryUrl":"https://github.com/metorial/metorial-node","directory":"examples/typescript-openai","name":"Metorial + OpenAI","description":"Example description","sdk":"metorial-node"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	restore := overrideExampleTestGlobals(server.URL+"/cli/manifest.json", server.URL, server.URL, server.Client(), func(file string) (string, error) {
		return "", errors.New("not found")
	})
	defer restore()

	stdout := &bytes.Buffer{}
	command := NewCommand(commandutil.NewContext(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}}, &commandutil.RootOptions{Format: "structured"}))

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rendered := stdout.String()
	if !strings.Contains(rendered, "Available Examples") {
		t.Fatalf("list output = %q", rendered)
	}
	if !strings.Contains(rendered, "metorial-openai") {
		t.Fatalf("list output missing identifier: %q", rendered)
	}
}

func TestExampleCreateJSONDownloadsPreparesAndInstalls(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	targetDir := filepath.Join(tempDir, "project")
	npmDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(npmDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}

	npmPath := filepath.Join(npmDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho resolving packages\necho install complete\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(npm) error = %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", npmDir+string(os.PathListSeparator)+originalPath)

	archive := buildExampleArchive(t, map[string]string{
		"metorial-node-main/examples/typescript-openai/package.json": `{"name":"example"}`,
		"metorial-node-main/examples/typescript-openai/.env.example": "METORIAL_API_KEY={your_api_key_here}\n",
		"metorial-node-main/examples/typescript-openai/src/index.ts": "console.log('hello')\n",
	})

	store, err := config.OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProfile(config.Profile{
		ID:          "org_1:usr_1",
		Name:        "demo",
		APIHost:     "",
		AccessToken: "oauth_access_token",
		OrgID:       "org_1",
		OrgName:     "Demo Org",
		UserID:      "usr_1",
		UserName:    "Demo User",
		UserEmail:   "demo@example.com",
	}, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cli/manifest.json":
			_, _ = writer.Write([]byte(`{"sdks":[{"id":"metorial-node","name":"Metorial Node"}],"examples":[{"repositoryUrl":"https://github.com/metorial/metorial-node","directory":"examples/typescript-openai","name":"Metorial + OpenAI","description":"Example description","sdk":"metorial-node"}]}`))
		case "/instances":
			_, _ = writer.Write([]byte(`{"object":"list","items":[{"object":"organization.instance","id":"inst_123","slug":"development","name":"Development","organization_id":"org_1","type":"development","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","project":{"object":"organization.project","id":"proj_1","status":"active","slug":"demo","name":"Demo","organization_id":"org_1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}}]}`))
		case "/organization/api-keys":
			if request.Method != http.MethodPost {
				t.Fatalf("api key request method = %s, want POST", request.Method)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("io.ReadAll(request.Body) error = %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("json.Unmarshal(body) error = %v", err)
			}
			if payload["type"] != "instance_access_token_secret" {
				t.Fatalf("payload[type] = %#v", payload["type"])
			}
			if payload["instance_id"] != "inst_123" {
				t.Fatalf("payload[instance_id] = %#v, want inst_123", payload["instance_id"])
			}
			if payload["name"] != "Demo User - Metorial + OpenAI" {
				t.Fatalf("payload[name] = %#v", payload["name"])
			}
			_, _ = writer.Write([]byte(`{"secret":"metorial_sk_created"}`))
		case "/repos/metorial/metorial-node":
			_, _ = writer.Write([]byte(`{"default_branch":"main"}`))
		case "/metorial/metorial-node/zip/refs/heads/main":
			writer.Header().Set("Content-Type", "application/zip")
			_, _ = writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	restore := overrideExampleTestGlobals(server.URL+"/cli/manifest.json", server.URL, server.URL, server.Client(), func(file string) (string, error) {
		return exec.LookPath(file)
	})
	defer restore()

	stdout := &bytes.Buffer{}
	command := NewCommand(commandutil.NewContext(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}}, &commandutil.RootOptions{
		APIHost: server.URL,
		Format:  "json",
	}))

	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"create", "metorial-openai", targetDir})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var result exampleCreateResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal(output) error = %v\noutput: %s", err, stdout.String())
	}

	if result.Identifier != "metorial-openai" {
		t.Fatalf("result.Identifier = %q, want %q", result.Identifier, "metorial-openai")
	}
	if result.Install == nil || result.Install.Kind != "npm" {
		t.Fatalf("result.Install = %#v, want npm install plan", result.Install)
	}
	if result.InstallLastOutput != "install complete" {
		t.Fatalf("result.InstallLastOutput = %q, want %q", result.InstallLastOutput, "install complete")
	}
	if result.APIKeyProvision == nil || !result.APIKeyProvision.Created {
		t.Fatalf("result.APIKeyProvision = %#v, want created key", result.APIKeyProvision)
	}

	envContent, err := os.ReadFile(filepath.Join(targetDir, ".env"))
	if err != nil {
		t.Fatalf(".env missing: %v", err)
	}
	if !strings.Contains(string(envContent), "METORIAL_API_KEY=metorial_sk_created") {
		t.Fatalf(".env content = %q", string(envContent))
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".env.example")); !os.IsNotExist(err) {
		t.Fatalf(".env.example should have been renamed, err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "src", "index.ts")); err != nil {
		t.Fatalf("src/index.ts missing: %v", err)
	}
}

func TestReplaceExampleAPIKeyPlaceholderSupportsEnvVariants(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".env.local")
	if err := os.WriteFile(envPath, []byte("export METORIAL_API_KEY=\"your-metorial-api-key\" # fill me in\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env.local) error = %v", err)
	}

	files, err := findExampleAPIKeyPlaceholderFiles(root)
	if err != nil {
		t.Fatalf("findExampleAPIKeyPlaceholderFiles() error = %v", err)
	}
	if len(files) != 1 || files[0] != ".env.local" {
		t.Fatalf("files = %#v, want [.env.local]", files)
	}

	updated, err := replaceExampleAPIKeyPlaceholders(root, files, "metorial_sk_created")
	if err != nil {
		t.Fatalf("replaceExampleAPIKeyPlaceholders() error = %v", err)
	}
	if len(updated) != 1 || updated[0] != ".env.local" {
		t.Fatalf("updated = %#v, want [.env.local]", updated)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(.env.local) error = %v", err)
	}
	if !strings.Contains(string(content), "export METORIAL_API_KEY=\"metorial_sk_created\" # fill me in") {
		t.Fatalf("content = %q", string(content))
	}
}

func TestExampleAPIKeyPlaceholderDetectionSupportsCommonTemplateValues(t *testing.T) {
	cases := []string{
		"METORIAL_API_KEY=your-metorial-api-key\n",
		"METORIAL_API_KEY=your_metorial_api_key_here\n",
		"export METORIAL_API_KEY=\"{your_api_key_here}\"\n",
	}

	for _, content := range cases {
		if !containsExampleAPIKeyPlaceholder(content) {
			t.Fatalf("containsExampleAPIKeyPlaceholder(%q) = false, want true", content)
		}
	}

	if containsExampleAPIKeyPlaceholder("METORIAL_API_KEY=metorial_sk_real\n") {
		t.Fatalf("real API key should not be treated as a placeholder")
	}
}

func overrideExampleTestGlobals(manifestURL, apiURL, codeloadURL string, client *http.Client, lookPath func(string) (string, error)) func() {
	originalManifestURL := examplesManifestURL
	originalAPIURL := githubAPIBaseURL
	originalCodeloadURL := githubCodeloadBaseURL
	originalClient := exampleHTTPClient
	originalLookPath := exampleLookPath

	examplesManifestURL = manifestURL
	githubAPIBaseURL = apiURL
	githubCodeloadBaseURL = codeloadURL
	exampleHTTPClient = client
	exampleLookPath = lookPath

	return func() {
		examplesManifestURL = originalManifestURL
		githubAPIBaseURL = originalAPIURL
		githubCodeloadBaseURL = originalCodeloadURL
		exampleHTTPClient = originalClient
		exampleLookPath = originalLookPath
	}
}

func buildExampleArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("writer.Create(%q) error = %v", name, err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("fileWriter.Write(%q) error = %v", name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	return buffer.Bytes()
}
