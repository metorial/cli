package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/metorial/cli/internal/version"
	"golang.org/x/mod/semver"
)

const (
	EnvInstallMethod   = "METORIAL_INSTALL_METHOD"
	EnvInstallPM       = "METORIAL_INSTALL_PM"
	EnvInstallPackage  = "METORIAL_INSTALL_PACKAGE"
	EnvSkipUpdateCheck = "METORIAL_SKIP_UPDATE_CHECK"

	installMethodUnknown    = "unknown"
	installMethodInstallSH  = "install_sh"
	installMethodNPM        = "npm"
	installMethodHomebrew   = "homebrew"
	installMethodScoop      = "scoop"
	installMethodChocolatey = "chocolatey"

	updateCheckInterval      = 2 * time.Hour
	latestVersionEndpointURL = "https://cli.metorial.com/metorial-cli/latest"
)

type InstallInfo struct {
	Method            string `json:"method"`
	PackageManager    string `json:"package_manager,omitempty"`
	PackageName       string `json:"package_name,omitempty"`
	BinDir            string `json:"bin_dir,omitempty"`
	SymlinkPath       string `json:"symlink_path,omitempty"`
	ManagedBinaryPath string `json:"managed_binary_path,omitempty"`
}

type CheckState struct {
	LastCheckedAt time.Time `json:"last_checked_at"`
	LatestVersion string    `json:"latest_version"`
}

type Notice struct {
	CurrentVersion string
	LatestVersion  string
	Install        InstallInfo
}

func MaybePrintUpgradeNotice(writer io.Writer, features terminal.Features) error {
	if strings.TrimSpace(os.Getenv(EnvSkipUpdateCheck)) != "" {
		return nil
	}

	notice, err := GetUpgradeNotice()
	if err != nil || notice == nil {
		return nil
	}

	renderUpgradeNotice(writer, features, *notice)
	return nil
}

func GetUpgradeNotice() (*Notice, error) {
	current := normalizeSemver(version.Version)
	if current == "" {
		return nil, nil
	}

	install, err := DetectInstall()
	if err != nil {
		return nil, err
	}

	state, err := LoadOrRefreshCheckState()
	if err != nil {
		return nil, err
	}

	latest := normalizeSemver(state.LatestVersion)
	if latest == "" || semver.Compare(current, latest) >= 0 {
		return nil, nil
	}

	return &Notice{
		CurrentVersion: current,
		LatestVersion:  latest,
		Install:        install,
	}, nil
}

func DetectInstall() (InstallInfo, error) {
	if env := detectInstallFromEnv(); env.Method != installMethodUnknown {
		return env, nil
	}

	if metadata, err := readInstallMetadata(); err == nil && strings.TrimSpace(metadata.Method) != "" {
		return metadata, nil
	}

	return detectInstallFromExecutable()
}

func LoadOrRefreshCheckState() (CheckState, error) {
	state, err := readCheckState()
	if err == nil && time.Since(state.LastCheckedAt) < updateCheckInterval && strings.TrimSpace(state.LatestVersion) != "" {
		return state, nil
	}

	latest, fetchErr := fetchLatestVersion()
	if fetchErr != nil {
		if err == nil && strings.TrimSpace(state.LatestVersion) != "" {
			return state, nil
		}

		return CheckState{}, fetchErr
	}

	state = CheckState{
		LastCheckedAt: time.Now().UTC(),
		LatestVersion: latest,
	}
	_ = writeJSONFile(checkStatePath(), state)

	return state, nil
}

func Upgrade(stdout io.Writer, stderr io.Writer) error {
	install, err := DetectInstall()
	if err != nil {
		return err
	}

	command, args, supported := upgradeCommand(install)
	if !supported {
		if hint := upgradeHint(install); hint != "" {
			return fmt.Errorf("metorial: automatic upgrade is not available for this installation.\nUse %q instead.", hint)
		}

		return fmt.Errorf("metorial: unable to determine how to upgrade this installation")
	}

	process := exec.Command(command, args...)
	process.Stdout = stdout
	process.Stderr = stderr
	process.Stdin = os.Stdin

	if err := process.Run(); err != nil {
		return fmt.Errorf("metorial: upgrade failed: %w", err)
	}

	_ = os.Remove(checkStatePath())
	return nil
}

func renderUpgradeNotice(writer io.Writer, features terminal.Features, notice Notice) {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Metorial CLI update available"),
		fmt.Sprintf("%s %s", colors.Muted("Installed:"), colors.Bold(notice.CurrentVersion)),
		fmt.Sprintf("%s %s", colors.Muted("Latest:"), colors.Success(notice.LatestVersion)),
	}

	if hint := upgradeHint(notice.Install); hint != "" {
		lines = append(lines, fmt.Sprintf("%s %s", colors.Muted("Upgrade:"), colors.Notice(hint)))
	}

	_ = output.RenderBox(writer, lines, output.BoxOptions{
		MaxWidth: features.Width,
		Unicode:  features.HasUnicode,
	})
	_, _ = fmt.Fprintln(writer)
}

func detectInstallFromEnv() InstallInfo {
	method := strings.TrimSpace(os.Getenv(EnvInstallMethod))
	if method == "" {
		return InstallInfo{Method: installMethodUnknown}
	}

	return InstallInfo{
		Method:         method,
		PackageManager: strings.TrimSpace(os.Getenv(EnvInstallPM)),
		PackageName:    strings.TrimSpace(os.Getenv(EnvInstallPackage)),
	}
}

func readInstallMetadata() (InstallInfo, error) {
	var metadata InstallInfo
	if err := readJSONFile(installMetadataPath(), &metadata); err != nil {
		return InstallInfo{}, err
	}

	return metadata, nil
}

func detectInstallFromExecutable() (InstallInfo, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return InstallInfo{Method: installMethodUnknown}, nil
	}

	resolvedPath := executablePath
	if evalPath, evalErr := filepath.EvalSymlinks(executablePath); evalErr == nil {
		resolvedPath = evalPath
	}

	candidates := []string{filepath.ToSlash(executablePath), filepath.ToSlash(resolvedPath)}
	for _, candidate := range candidates {
		lower := strings.ToLower(candidate)

		switch {
		case strings.Contains(candidate, "/Cellar/metorial/"), strings.Contains(candidate, "/homebrew/opt/metorial/"), strings.Contains(candidate, "/linuxbrew/Cellar/metorial/"):
			return InstallInfo{Method: installMethodHomebrew}, nil
		case strings.Contains(lower, "/scoop/apps/metorial/"):
			return InstallInfo{Method: installMethodScoop}, nil
		case strings.Contains(lower, "/chocolatey/"):
			return InstallInfo{Method: installMethodChocolatey}, nil
		case strings.Contains(candidate, "/.metorial/cli/"):
			return InstallInfo{Method: installMethodInstallSH}, nil
		}
	}

	return InstallInfo{Method: installMethodUnknown}, nil
}

func upgradeHint(install InstallInfo) string {
	switch install.Method {
	case installMethodInstallSH, installMethodNPM:
		return "metorial upgrade"
	case installMethodHomebrew:
		return "brew upgrade metorial"
	case installMethodScoop:
		return "scoop update metorial"
	case installMethodChocolatey:
		return "choco upgrade metorial"
	default:
		return ""
	}
}

func upgradeCommand(install InstallInfo) (string, []string, bool) {
	switch install.Method {
	case installMethodInstallSH:
		return installSHUpgradeCommand(install)
	case installMethodNPM:
		return npmUpgradeCommand(install)
	default:
		return "", nil, false
	}
}

func installSHUpgradeCommand(install InstallInfo) (string, []string, bool) {
	binDir := strings.TrimSpace(install.BinDir)
	if binDir == "" {
		return "", nil, false
	}

	command := fmt.Sprintf("curl -fsSL %q | METORIAL_CLI_BIN_DIR=%q bash", "https://cli.metorial.com/install.sh", binDir)
	return "sh", []string{"-c", command}, true
}

func npmUpgradeCommand(install InstallInfo) (string, []string, bool) {
	packageName := strings.TrimSpace(install.PackageName)
	if packageName == "" {
		packageName = "@metorial/cli"
	}

	switch install.PackageManager {
	case "bun":
		return "bun", []string{"add", "-g", packageName + "@latest"}, true
	case "pnpm":
		return "pnpm", []string{"add", "-g", packageName + "@latest"}, true
	case "yarn":
		return "yarn", []string{"global", "add", packageName + "@latest"}, true
	default:
		return "npm", []string{"install", "-g", packageName + "@latest"}, true
	}
}

func fetchLatestVersion() (string, error) {
	response, err := http.Get(latestVersionEndpointURL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("metorial: update check failed with status %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

func readCheckState() (CheckState, error) {
	var state CheckState
	if err := readJSONFile(checkStatePath(), &state); err != nil {
		return CheckState{}, err
	}

	return state, nil
}

func readJSONFile(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, target)
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	return os.WriteFile(path, content, 0o600)
}

func installMetadataPath() string {
	return filepath.Join(cliDirPath(), "install.json")
}

func checkStatePath() string {
	return filepath.Join(cliDirPath(), "update-check.json")
}

func cliDirPath() string {
	cliDir, err := config.DefaultCLIDir()
	if err == nil {
		return cliDir
	}

	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return filepath.Join(".metorial", "cli")
	}

	return filepath.Join(homeDir, ".metorial", "cli")
}

func normalizeSemver(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	if !strings.HasPrefix(normalized, "v") {
		normalized = "v" + normalized
	}
	if !semver.IsValid(normalized) {
		return ""
	}

	return normalized
}
