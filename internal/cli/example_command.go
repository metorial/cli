package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	metorial "github.com/metorial/metorial-go/v1"
	"github.com/spf13/cobra"
)

var examplesManifestURL = "https://metorial.com/cli/manifest.json"
var githubAPIBaseURL = "https://api.github.com"
var githubCodeloadBaseURL = "https://codeload.github.com"
var exampleHTTPClient = &http.Client{Timeout: 60 * time.Second}
var exampleLookPath = exec.LookPath
var metorialAPIKeyLinePattern = regexp.MustCompile(`(?m)^(\s*(?:export\s+)?METORIAL_API_KEY\s*=\s*)(["']?)([^#\r\n]*?)(["']?)(\s*(?:#.*)?)$`)

type exampleManifest struct {
	SDKs     []exampleSDK          `json:"sdks"`
	Examples []exampleManifestItem `json:"examples"`
}

type exampleSDK struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	GitHubURL   string `json:"githubUrl"`
	Description string `json:"description"`
}

type exampleManifestItem struct {
	RepositoryURL string `json:"repositoryUrl"`
	Directory     string `json:"directory"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SDK           string `json:"sdk"`
}

type exampleRecord struct {
	Identifier    string `json:"identifier"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SDK           string `json:"sdk"`
	SDKName       string `json:"sdk_name"`
	RepositoryURL string `json:"repository_url"`
	Directory     string `json:"directory"`
	DefaultPath   string `json:"default_path"`
}

type exampleRepoInfo struct {
	DefaultBranch string `json:"default_branch"`
}

type exampleInstallPlan struct {
	Kind    string   `json:"kind"`
	Command []string `json:"command"`
}

type exampleCreateResult struct {
	Identifier        string              `json:"identifier"`
	Name              string              `json:"name"`
	SDK               string              `json:"sdk"`
	SDKName           string              `json:"sdk_name"`
	RepositoryURL     string              `json:"repository_url"`
	Directory         string              `json:"directory"`
	TargetPath        string              `json:"target_path"`
	EnvFilesCreated   []string            `json:"env_files_created"`
	APIKeyProvision   *exampleAPIKeySetup `json:"api_key_provision,omitempty"`
	Install           *exampleInstallPlan `json:"install,omitempty"`
	InstallLastOutput string              `json:"install_last_output,omitempty"`
}

type exampleAPIKeySetup struct {
	PlaceholderFiles []string `json:"placeholder_files"`
	UpdatedFiles     []string `json:"updated_files,omitempty"`
	Created          bool     `json:"created"`
	RequiresManual   bool     `json:"requires_manual"`
	InstanceID       string   `json:"instance_id,omitempty"`
}

type exampleProgress struct {
	writer   io.Writer
	features terminal.Features
	colors   terminal.Colorizer
	lineOpen bool
}

type exampleStep struct {
	progress *exampleProgress
	title    string
}

type exampleLineWriter struct {
	mu     sync.Mutex
	buffer string
	last   string
	onLine func(string)
}

func newExampleCommand(application *app.App, rootOptions *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:     "example",
		Aliases: []string{"examples"},
		Short:   "List and clone Metorial examples",
		Long: strings.TrimSpace(`
Browse official Metorial examples from the public CLI manifest and clone one
into a local directory.

Use "metorial example list" to see available examples. Use
"metorial example create <identifier> [path]" to download an example, prepare
the target folder, copy .env.example to .env when available, and install
dependencies when a supported package manager is present.
`),
	}

	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List official Metorial examples",
		RunE: func(command *cobra.Command, args []string) error {
			manifest, err := fetchExampleManifest()
			if err != nil {
				return err
			}

			records := manifest.records()
			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"sdks":     manifest.SDKs,
					"examples": records,
				})
			}

			colors := terminal.NewColorizer(application.StdoutFeatures())
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Bold("Available Examples"))
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted("Use one of these identifiers with `metorial example create <identifier>`."))
			_, _ = fmt.Fprintln(command.OutOrStdout())

			table := output.Table{
				Columns: []string{
					colors.Accent("Example"),
					colors.Accent("SDK"),
					colors.Accent("Description"),
				},
				Features: application.StdoutFeatures(),
				MaxWidth: application.StdoutFeatures().Width,
			}

			for _, record := range records {
				table.Rows = append(table.Rows, []string{
					colors.Bold(record.Name) + "\n" + colors.Muted(record.Identifier) + "\n",
					record.SDKName,
					record.Description,
				})
			}

			if err := table.Render(command.OutOrStdout()); err != nil {
				return err
			}

			if len(records) > 0 {
				_, _ = fmt.Fprintln(command.OutOrStdout())
				_, _ = fmt.Fprintf(command.OutOrStdout(), "%s `metorial example create %s`\n", colors.Notice("Clone an example with"), records[0].Identifier)
			}

			return nil
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "create <identifier> [path]",
		Short: "Clone an official Metorial example",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			var progress *exampleProgress
			if format == output.FormatStructured {
				progress = newExampleProgress(command.OutOrStdout(), application.StdoutFeatures())
			}

			result, err := runExampleCreate(application, rootOptions, args[0], optionalArg(args, 1), progress)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, result)
			}

			renderExampleCreateSummary(command.OutOrStdout(), application.StdoutFeatures(), result)
			return nil
		},
	})

	return command
}

func fetchExampleManifest() (*exampleManifest, error) {
	request, err := http.NewRequest(http.MethodGet, examplesManifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to create manifest request: %w", err)
	}

	request.Header.Set("User-Agent", "metorial-cli")

	response, err := exampleHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to fetch example manifest: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("metorial: failed to fetch example manifest: %s", response.Status)
	}

	var manifest exampleManifest
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("metorial: failed to decode example manifest: %w", err)
	}

	return &manifest, nil
}

func (m *exampleManifest) records() []exampleRecord {
	if m == nil {
		return nil
	}

	sdkByID := map[string]exampleSDK{}
	for _, sdk := range m.SDKs {
		sdkByID[sdk.ID] = sdk
	}

	baseCounts := map[string]int{}
	for _, item := range m.Examples {
		baseCounts[slugify(item.Name)]++
	}

	identifierCounts := map[string]int{}
	records := make([]exampleRecord, 0, len(m.Examples))
	for _, item := range m.Examples {
		sdk := sdkByID[item.SDK]
		identifier := slugify(item.Name)
		if identifier == "" {
			identifier = slugify(path.Base(strings.TrimSpace(item.Directory)))
		}
		if identifier == "" {
			identifier = "example"
		}

		if baseCounts[identifier] > 1 {
			directorySuffix := slugify(path.Base(strings.Trim(strings.TrimSpace(item.Directory), "/")))
			if directorySuffix != "" && directorySuffix != identifier {
				identifier = identifier + "-" + directorySuffix
			} else if slugify(item.SDK) != "" {
				identifier = identifier + "-" + slugify(item.SDK)
			}
		}

		identifierCounts[identifier]++
		if identifierCounts[identifier] > 1 {
			identifier = fmt.Sprintf("%s-%d", identifier, identifierCounts[identifier])
		}

		records = append(records, exampleRecord{
			Identifier:    identifier,
			Name:          item.Name,
			Description:   item.Description,
			SDK:           item.SDK,
			SDKName:       firstNonEmpty(sdk.Name, item.SDK),
			RepositoryURL: item.RepositoryURL,
			Directory:     item.Directory,
			DefaultPath:   exampleDirectorySlug(item.Name),
		})
	}

	sort.Slice(records, func(left, right int) bool {
		if records[left].SDKName == records[right].SDKName {
			return records[left].Name < records[right].Name
		}
		return records[left].SDKName < records[right].SDKName
	})

	return records
}

func runExampleCreate(application *app.App, rootOptions *rootOptions, identifier, requestedPath string, progress *exampleProgress) (*exampleCreateResult, error) {
	manifest, err := fetchExampleManifest()
	if err != nil {
		return nil, err
	}

	record, err := findExampleRecord(manifest.records(), identifier)
	if err != nil {
		return nil, err
	}

	targetPath := strings.TrimSpace(requestedPath)
	if targetPath == "" {
		targetPath = record.DefaultPath
	}

	absoluteTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to resolve target path: %w", err)
	}

	targetStep := progress.Start("Preparing target directory")
	if err := ensureEmptyDirectory(absoluteTargetPath); err != nil {
		targetStep.Fail()
		return nil, err
	}
	targetStep.Success("Prepared target directory")

	owner, repo, err := parseGitHubRepository(record.RepositoryURL)
	if err != nil {
		return nil, err
	}

	repoStep := progress.Start("Resolving repository archive")
	defaultBranch, err := fetchGitHubDefaultBranch(owner, repo)
	if err != nil {
		repoStep.Fail()
		return nil, err
	}
	repoStep.Success(fmt.Sprintf("Resolved %s/%s@%s", owner, repo, defaultBranch))

	downloadStep := progress.Start("Downloading example files")
	archiveBytes, err := downloadGitHubArchive(owner, repo, defaultBranch)
	if err != nil {
		downloadStep.Fail()
		return nil, err
	}
	downloadStep.Success("Downloaded repository archive")

	extractStep := progress.Start("Copying example into place")
	if err := extractExampleArchive(archiveBytes, record.Directory, absoluteTargetPath); err != nil {
		extractStep.Fail()
		return nil, err
	}
	extractStep.Success("Copied example files")

	envStep := progress.Start("Preparing environment files")
	envFiles, err := renameEnvExampleFiles(absoluteTargetPath)
	if err != nil {
		envStep.Fail()
		return nil, err
	}
	if len(envFiles) == 0 {
		envStep.Success("No .env.example files found")
	} else {
		envStep.Success(fmt.Sprintf("Created %d .env file(s)", len(envFiles)))
	}

	result := &exampleCreateResult{
		Identifier:      record.Identifier,
		Name:            record.Name,
		SDK:             record.SDK,
		SDKName:         record.SDKName,
		RepositoryURL:   record.RepositoryURL,
		Directory:       record.Directory,
		TargetPath:      absoluteTargetPath,
		EnvFilesCreated: envFiles,
	}

	apiKeyStep := progress.Start("Preparing Metorial API key")
	apiKeySetup, err := provisionExampleAPIKey(application, rootOptions, record, absoluteTargetPath)
	if err != nil {
		apiKeyStep.Fail()
		return nil, err
	}
	result.APIKeyProvision = apiKeySetup
	switch {
	case apiKeySetup == nil:
		apiKeyStep.Success("No METORIAL_API_KEY placeholder found")
	case apiKeySetup.Created:
		apiKeyStep.Success("Created a Metorial API key and updated .env")
	case apiKeySetup.RequiresManual:
		apiKeyStep.Success("METORIAL_API_KEY still needs to be filled in")
	default:
		apiKeyStep.Success("Prepared Metorial API key configuration")
	}

	installPlan, err := detectExampleInstall(absoluteTargetPath)
	if err != nil {
		return nil, err
	}
	if installPlan == nil {
		progress.Start("Installing dependencies").Success("No supported dependency installer found")
		return result, nil
	}

	result.Install = installPlan
	installStep := progress.Start("Installing dependencies")
	lastLine, err := runExampleInstall(context.Background(), absoluteTargetPath, installPlan, installStep)
	if err != nil {
		installStep.Fail()
		return nil, err
	}
	result.InstallLastOutput = lastLine
	installStep.Success("Installed dependencies")

	return result, nil
}

func findExampleRecord(records []exampleRecord, identifier string) (*exampleRecord, error) {
	target := strings.TrimSpace(identifier)
	for _, record := range records {
		if record.Identifier == target {
			copy := record
			return &copy, nil
		}
	}

	repositoryTarget, directoryTarget, ok := parseExampleRepositoryTarget(target)
	if ok {
		for _, record := range records {
			repositoryName := exampleRepositoryName(record.RepositoryURL)
			if repositoryName == repositoryTarget && strings.Trim(strings.TrimSpace(record.Directory), "/") == directoryTarget {
				copy := record
				return &copy, nil
			}
		}
	}

	return nil, fmt.Errorf("metorial: example %q was not found.\nRun \"metorial example list\" to see available examples.", identifier)
}

func parseExampleRepositoryTarget(value string) (string, string, bool) {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "", false
	}

	repository := strings.TrimSpace(parts[0])
	directory := strings.Trim(strings.Join(parts[1:], "/"), "/")
	if repository == "" || directory == "" {
		return "", "", false
	}

	return repository, directory, true
}

func exampleRepositoryName(repositoryURL string) string {
	_, repository, err := parseGitHubRepository(repositoryURL)
	if err != nil {
		return ""
	}
	return repository
}

func ensureEmptyDirectory(targetPath string) error {
	info, err := os.Stat(targetPath)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("metorial: target path %q is not a directory", targetPath)
		}

		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return fmt.Errorf("metorial: failed to inspect target directory %q: %w", targetPath, err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("metorial: target directory %q must be empty", targetPath)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("metorial: failed to inspect target directory %q: %w", targetPath, err)
	}

	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return fmt.Errorf("metorial: failed to create target directory %q: %w", targetPath, err)
	}

	return nil
}

func parseGitHubRepository(repositoryURL string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(repositoryURL))
	if err != nil {
		return "", "", fmt.Errorf("metorial: invalid repository URL %q: %w", repositoryURL, err)
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("metorial: invalid repository URL %q", repositoryURL)
	}

	return segments[0], strings.TrimSuffix(segments[1], ".git"), nil
}

func fetchGitHubDefaultBranch(owner, repo string) (string, error) {
	requestURL := strings.TrimRight(githubAPIBaseURL, "/") + "/repos/" + owner + "/" + repo
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("metorial: failed to create repository request: %w", err)
	}

	request.Header.Set("User-Agent", "metorial-cli")

	response, err := exampleHTTPClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("metorial: failed to fetch repository metadata: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("metorial: failed to fetch repository metadata: %s", response.Status)
	}

	var repoInfo exampleRepoInfo
	if err := json.NewDecoder(response.Body).Decode(&repoInfo); err != nil {
		return "", fmt.Errorf("metorial: failed to decode repository metadata: %w", err)
	}
	if strings.TrimSpace(repoInfo.DefaultBranch) == "" {
		return "", fmt.Errorf("metorial: repository %s/%s does not have a default branch", owner, repo)
	}

	return repoInfo.DefaultBranch, nil
}

func downloadGitHubArchive(owner, repo, branch string) ([]byte, error) {
	requestURL := strings.TrimRight(githubCodeloadBaseURL, "/") + "/" + owner + "/" + repo + "/zip/refs/heads/" + url.PathEscape(branch)
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to create archive request: %w", err)
	}

	request.Header.Set("User-Agent", "metorial-cli")

	response, err := exampleHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to download example archive: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("metorial: failed to download example archive: %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to read example archive: %w", err)
	}

	return body, nil
}

func extractExampleArchive(archive []byte, exampleDirectory, targetPath string) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("metorial: failed to read example archive: %w", err)
	}

	directory := strings.Trim(path.Clean(strings.TrimSpace(exampleDirectory)), "/")
	if directory == "." || directory == "" {
		return fmt.Errorf("metorial: example directory is missing from the manifest")
	}

	found := false
	for _, file := range reader.File {
		parts := strings.SplitN(file.Name, "/", 2)
		if len(parts) != 2 {
			continue
		}

		repositoryRelativePath := parts[1]
		if repositoryRelativePath != directory && !strings.HasPrefix(repositoryRelativePath, directory+"/") {
			continue
		}

		found = true
		relativePath := strings.TrimPrefix(repositoryRelativePath, directory)
		relativePath = strings.TrimPrefix(relativePath, "/")
		if relativePath == "" {
			continue
		}

		destinationPath := filepath.Join(targetPath, filepath.FromSlash(relativePath))
		cleanDestinationPath := filepath.Clean(destinationPath)
		if cleanDestinationPath != targetPath && !strings.HasPrefix(cleanDestinationPath, targetPath+string(os.PathSeparator)) {
			return fmt.Errorf("metorial: invalid file path %q in example archive", relativePath)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanDestinationPath, 0o755); err != nil {
				return fmt.Errorf("metorial: failed to create directory %q: %w", cleanDestinationPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanDestinationPath), 0o755); err != nil {
			return fmt.Errorf("metorial: failed to create directory %q: %w", filepath.Dir(cleanDestinationPath), err)
		}

		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("metorial: failed to open archive file %q: %w", file.Name, err)
		}

		mode := file.Mode()
		if mode == 0 {
			mode = 0o644
		}

		destination, err := os.OpenFile(cleanDestinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			_ = source.Close()
			return fmt.Errorf("metorial: failed to create file %q: %w", cleanDestinationPath, err)
		}

		_, copyErr := io.Copy(destination, source)
		closeErr := destination.Close()
		sourceCloseErr := source.Close()
		if copyErr != nil {
			return fmt.Errorf("metorial: failed to extract file %q: %w", cleanDestinationPath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("metorial: failed to finalize file %q: %w", cleanDestinationPath, closeErr)
		}
		if sourceCloseErr != nil {
			return fmt.Errorf("metorial: failed to close archive file %q: %w", file.Name, sourceCloseErr)
		}
	}

	if !found {
		return fmt.Errorf("metorial: example directory %q was not found in the repository archive", exampleDirectory)
	}

	return nil
}

func renameEnvExampleFiles(root string) ([]string, error) {
	renamed := []string{}

	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != ".env.example" {
			return nil
		}

		targetPath := filepath.Join(filepath.Dir(filePath), ".env")
		if _, err := os.Stat(targetPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := os.Rename(filePath, targetPath); err != nil {
			return err
		}

		relativePath, err := filepath.Rel(root, targetPath)
		if err != nil {
			relativePath = targetPath
		}
		renamed = append(renamed, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to prepare environment files: %w", err)
	}

	sort.Strings(renamed)
	return renamed, nil
}

func provisionExampleAPIKey(application *app.App, rootOptions *rootOptions, record *exampleRecord, root string) (*exampleAPIKeySetup, error) {
	placeholderFiles, err := findExampleAPIKeyPlaceholderFiles(root)
	if err != nil {
		return nil, err
	}
	if len(placeholderFiles) == 0 {
		return nil, nil
	}

	setup := &exampleAPIKeySetup{
		PlaceholderFiles: placeholderFiles,
		RequiresManual:   true,
	}

	if application == nil || rootOptions == nil {
		return setup, nil
	}

	runtime, err := application.ResolveAuthConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile)
	if err != nil {
		return setup, nil
	}
	if runtime.Profile == nil || strings.TrimSpace(runtime.Profile.OrgID) == "" {
		return setup, nil
	}

	runtime, err = application.ResolveExampleInstance(runtime, rootOptions.instance)
	if err != nil {
		return setup, nil
	}
	if strings.TrimSpace(runtime.InstanceID) == "" {
		return setup, nil
	}

	apiKey, err := createExampleAPIKey(runtime, record.Name)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return setup, nil
	}
	apiKeyPreview := apiKey
	if len(apiKeyPreview) > 16 {
		apiKeyPreview = apiKeyPreview[:16]
	}

	updatedFiles, err := replaceExampleAPIKeyPlaceholders(root, placeholderFiles, apiKey)
	if err != nil {
		return nil, err
	}

	setup.Created = true
	setup.RequiresManual = false
	setup.InstanceID = runtime.InstanceID
	setup.UpdatedFiles = updatedFiles
	return setup, nil
}

func findExampleAPIKeyPlaceholderFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".env") {
			return nil
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if !containsExampleAPIKeyPlaceholder(string(content)) {
			return nil
		}

		relativePath, err := filepath.Rel(root, filePath)
		if err != nil {
			relativePath = filePath
		}
		files = append(files, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to inspect .env files: %w", err)
	}

	sort.Strings(files)
	return files, nil
}

func replaceExampleAPIKeyPlaceholders(root string, files []string, apiKey string) ([]string, error) {
	updated := []string{}
	for _, relativePath := range files {
		absolutePath := filepath.Join(root, filepath.FromSlash(relativePath))
		content, err := os.ReadFile(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read %q: %w", absolutePath, err)
		}

		replaced := replaceExampleAPIKeyPlaceholderContent(string(content), apiKey)
		if replaced == string(content) {
			continue
		}

		if err := os.WriteFile(absolutePath, []byte(replaced), 0o600); err != nil {
			return nil, fmt.Errorf("metorial: failed to write %q: %w", absolutePath, err)
		}
		updated = append(updated, relativePath)
	}

	sort.Strings(updated)
	return updated, nil
}

func containsExampleAPIKeyPlaceholder(content string) bool {
	matches := metorialAPIKeyLinePattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		if isExampleAPIKeyPlaceholder(match[3]) {
			return true
		}
	}
	return false
}

func replaceExampleAPIKeyPlaceholderContent(content, apiKey string) string {
	return metorialAPIKeyLinePattern.ReplaceAllStringFunc(content, func(line string) string {
		match := metorialAPIKeyLinePattern.FindStringSubmatch(line)
		if len(match) < 6 {
			return line
		}
		if !isExampleAPIKeyPlaceholder(match[3]) {
			return line
		}
		return match[1] + match[2] + apiKey + match[4] + match[5]
	})
}

func isExampleAPIKeyPlaceholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, `"'`)
	normalized := strings.ToLower(strings.TrimSpace(trimmed))
	if normalized == "" {
		return false
	}
	if strings.HasPrefix(normalized, "metorial_sk_") || strings.HasPrefix(normalized, "metorial_fk_") || strings.HasPrefix(normalized, "metorial_pk") {
		return false
	}
	if strings.HasPrefix(normalized, "{") && strings.HasSuffix(normalized, "}") {
		return true
	}

	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "your_metorial_api_key", "your_metorial_api_key_here", "metorial_api_key_here":
		return true
	}

	return strings.Contains(normalized, "metorial") && strings.Contains(normalized, "api_key") && strings.Contains(normalized, "your")
}

func createExampleAPIKey(runtime config.Runtime, exampleName string) (string, error) {
	if runtime.Profile == nil {
		return "", fmt.Errorf("metorial: a current profile is required")
	}

	userName := firstNonEmpty(runtime.Profile.UserName, runtime.Profile.UserEmail, runtime.Profile.UserID)
	if strings.TrimSpace(userName) == "" || strings.TrimSpace(runtime.Profile.OrgID) == "" || strings.TrimSpace(runtime.InstanceID) == "" {
		return "", fmt.Errorf("metorial: API key creation requires a profile, organization, and instance")
	}

	sdk, err := runtime.SDK()
	if err != nil {
		return "", err
	}

	requestURL := runtime.APIHostURL.ResolveReference(&url.URL{Path: "/organization/api-keys"}).String()
	payload, err := json.Marshal(map[string]any{
		"type":        "instance_access_token_secret",
		"instance_id": runtime.InstanceID,
		"name":        userName + " - " + strings.TrimSpace(exampleName),
	})
	if err != nil {
		return "", fmt.Errorf("metorial: failed to encode API key request: %w", err)
	}

	response, err := sdk.Fetch(&metorial.RawRequest{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: payload,
	})
	if err != nil {
		return "", err
	}
	if response == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("metorial: API key creation failed")
	}

	var decoded struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(response.Body, &decoded); err != nil {
		return "", err
	}
	if strings.TrimSpace(decoded.Secret) == "" {
		return "", fmt.Errorf("metorial: API key secret missing from response")
	}

	return strings.TrimSpace(decoded.Secret), nil
}

func detectExampleInstall(root string) (*exampleInstallPlan, error) {
	packageJSONPath := filepath.Join(root, "package.json")
	if info, err := os.Stat(packageJSONPath); err == nil && !info.IsDir() {
		if npmPath, err := exampleLookPath("npm"); err == nil && strings.TrimSpace(npmPath) != "" {
			return &exampleInstallPlan{
				Kind:    "npm",
				Command: []string{"npm", "install"},
			}, nil
		}
	}

	requirementsPath := filepath.Join(root, "requirements.txt")
	if info, err := os.Stat(requirementsPath); err == nil && !info.IsDir() {
		if pipPath, err := exampleLookPath("pip"); err == nil && strings.TrimSpace(pipPath) != "" {
			return &exampleInstallPlan{
				Kind:    "pip",
				Command: []string{"pip", "install", "-r", "requirements.txt"},
			}, nil
		}
		if pipPath, err := exampleLookPath("pip3"); err == nil && strings.TrimSpace(pipPath) != "" {
			return &exampleInstallPlan{
				Kind:    "pip",
				Command: []string{"pip3", "install", "-r", "requirements.txt"},
			}, nil
		}
	}

	return nil, nil
}

func runExampleInstall(ctx context.Context, root string, plan *exampleInstallPlan, step *exampleStep) (string, error) {
	if plan == nil || len(plan.Command) == 0 {
		return "", nil
	}

	command := exec.CommandContext(ctx, plan.Command[0], plan.Command[1:]...)
	command.Dir = root

	writer := &exampleLineWriter{
		onLine: func(line string) {
			if line != "" {
				step.Preview(line)
			}
		},
	}
	command.Stdout = writer
	command.Stderr = writer

	err := command.Run()
	lastLine := writer.Flush()
	if err != nil {
		if strings.TrimSpace(lastLine) != "" {
			return lastLine, fmt.Errorf("metorial: %s failed: %w\nLast output: %s", strings.Join(plan.Command, " "), err, lastLine)
		}
		return "", fmt.Errorf("metorial: %s failed: %w", strings.Join(plan.Command, " "), err)
	}

	return lastLine, nil
}

func (w *exampleLineWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	normalized := strings.ReplaceAll(string(payload), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	w.buffer += normalized

	for {
		index := strings.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := strings.TrimSpace(w.buffer[:index])
		w.buffer = w.buffer[index+1:]
		if line != "" && w.onLine != nil {
			w.last = line
			w.onLine(line)
		} else if line != "" {
			w.last = line
		}
	}

	return len(payload), nil
}

func (w *exampleLineWriter) Flush() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	line := strings.TrimSpace(w.buffer)
	w.buffer = ""
	if line != "" && w.onLine != nil {
		w.last = line
		w.onLine(line)
	} else if line != "" {
		w.last = line
	}
	return w.last
}

func newExampleProgress(writer io.Writer, features terminal.Features) *exampleProgress {
	return &exampleProgress{
		writer:   writer,
		features: features,
		colors:   terminal.NewColorizer(features),
	}
}

func (p *exampleProgress) Start(title string) *exampleStep {
	if p == nil {
		return &exampleStep{}
	}

	if p.features.IsTTY {
		p.closeLine(false)
		_, _ = fmt.Fprintf(p.writer, "\r\033[2K%s %s", p.colors.Notice("•"), title)
		p.lineOpen = true
	} else {
		_, _ = fmt.Fprintf(p.writer, "%s %s\n", p.colors.Notice("•"), title)
	}
	return &exampleStep{progress: p, title: title}
}

func (s *exampleStep) Preview(message string) {
	if s == nil || s.progress == nil {
		return
	}

	progress := s.progress
	if !progress.features.IsTTY {
		return
	}

	progress.lineOpen = true
	_, _ = fmt.Fprintf(
		progress.writer,
		"\r\033[2K%s %s %s",
		progress.colors.Notice("•"),
		s.title,
		progress.colors.Muted(compactExamplePreview(message, progress.features.Width)),
	)
}

func (s *exampleStep) Success(message string) {
	if s == nil || s.progress == nil {
		return
	}

	s.progress.closeLine(false)
	_, _ = fmt.Fprintf(s.progress.writer, "%s %s\n", s.progress.colors.Success("✓"), message)
}

func (s *exampleStep) Fail() {
	if s == nil || s.progress == nil {
		return
	}

	s.progress.closeLine(false)
	_, _ = fmt.Fprintf(s.progress.writer, "%s %s\n", s.progress.colors.Warning("!"), s.title)
}

func (p *exampleProgress) closeLine(addNewline bool) {
	if p == nil || !p.lineOpen {
		return
	}

	_, _ = fmt.Fprint(p.writer, "\r\033[2K")
	if addNewline {
		_, _ = fmt.Fprintln(p.writer)
	}
	p.lineOpen = false
}

func compactExamplePreview(value string, width int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if text == "" {
		return ""
	}

	if width <= 0 {
		width = 100
	}

	maxWidth := width - 4
	if maxWidth < 20 {
		maxWidth = 20
	}
	if len(text) <= maxWidth {
		return text
	}

	return text[:maxWidth-3] + "..."
}

func renderExampleCreateSummary(writer io.Writer, features terminal.Features, result *exampleCreateResult) {
	if result == nil {
		return
	}

	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Bold("Example Ready"))
	_, _ = fmt.Fprintln(writer)

	items := []output.DataListItem{
		{Label: "Example", Value: result.Name},
		{Label: "Identifier", Value: result.Identifier},
		{Label: "SDK", Value: result.SDKName},
		{Label: "Location", Value: result.TargetPath},
	}
	if result.Install != nil {
		items = append(items, output.DataListItem{
			Label: "Install",
			Value: strings.Join(result.Install.Command, " "),
		})
	}

	_ = output.RenderDataList(writer, items)

	if len(result.EnvFilesCreated) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Notice("Environment"))
		for _, filePath := range result.EnvFilesCreated {
			_, _ = fmt.Fprintf(writer, "  %s\n", filePath)
		}
	}

	if result.APIKeyProvision != nil {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Notice("API Key"))
		if result.APIKeyProvision.Created {
			_, _ = fmt.Fprintln(writer, "  Created a new Metorial API key and filled it into .env.")
		} else if result.APIKeyProvision.RequiresManual {
			_, _ = fmt.Fprintln(writer, "  Fill in METORIAL_API_KEY in .env before running the example.")
		}
	}

	if strings.TrimSpace(result.InstallLastOutput) != "" {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Muted("Last install output:"), result.InstallLastOutput)
	}
}

func exampleDirectorySlug(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = strings.ReplaceAll(slugPattern.ReplaceAllString(slug, "_"), "-", "_")
	slug = strings.Trim(slug, "_")
	if slug == "" {
		return "metorial_example"
	}
	return slug
}

func optionalArg(args []string, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	return args[index]
}
