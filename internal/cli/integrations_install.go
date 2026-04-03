package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	metorial "github.com/metorial/metorial-go/v1"
	"github.com/metorial/metorial-go/v1/endpoints"
	"github.com/metorial/metorial-go/v1/resources/magicmcpservers"
	"github.com/metorial/metorial-go/v1/resources/magicmcptokens"
	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

type installTransport string

const (
	installTransportSSE            installTransport = "sse"
	installTransportStreamableHTTP installTransport = "streamable_http"
)

type installPlanMethod string

const (
	installPlanMethodCommand installPlanMethod = "command"
	installPlanMethodFile    installPlanMethod = "file"
)

type remoteServerDefinition struct {
	Name      string
	URL       string
	Transport installTransport
}

type clientCapabilities struct {
	Transports []installTransport
}

type commandInstallPlan struct {
	Method    installPlanMethod `json:"method"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Transport installTransport  `json:"transport"`
}

type fileInstallPlan struct {
	Method    installPlanMethod `json:"method"`
	Format    string            `json:"format"`
	Path      string            `json:"path"`
	Content   map[string]any    `json:"content"`
	Transport installTransport  `json:"transport"`
}

type clientInstallPlan interface {
	method() installPlanMethod
	transport() installTransport
}

func (p commandInstallPlan) method() installPlanMethod   { return p.Method }
func (p commandInstallPlan) transport() installTransport { return p.Transport }
func (p fileInstallPlan) method() installPlanMethod      { return p.Method }
func (p fileInstallPlan) transport() installTransport    { return p.Transport }

type clientDetection struct {
	Installed bool   `json:"installed"`
	Usable    bool   `json:"usable"`
	Location  string `json:"location,omitempty"`
}

type clientAdapter interface {
	id() string
	label() string
	capabilities() clientCapabilities
	detect() clientDetection
	buildInstallPlan(server remoteServerDefinition) (clientInstallPlan, error)
}

type remoteCommandTemplate func(remoteCommandInput) []string

type remoteCommandInput struct {
	Name      string
	URL       string
	Transport installTransport
}

type commandClientAdapter struct {
	clientID           string
	clientLabel        string
	command            string
	template           remoteCommandTemplate
	clientCapabilities clientCapabilities
}

func (a commandClientAdapter) id() string                       { return a.clientID }
func (a commandClientAdapter) label() string                    { return a.clientLabel }
func (a commandClientAdapter) capabilities() clientCapabilities { return a.clientCapabilities }
func (a commandClientAdapter) detect() clientDetection {
	path, err := exec.LookPath(a.command)
	if err != nil {
		return clientDetection{}
	}
	return clientDetection{Installed: true, Usable: true, Location: path}
}
func (a commandClientAdapter) buildInstallPlan(server remoteServerDefinition) (clientInstallPlan, error) {
	return commandInstallPlan{
		Method:    installPlanMethodCommand,
		Command:   a.command,
		Args:      a.template(remoteCommandInput{Name: server.Name, URL: server.URL, Transport: server.Transport}),
		Transport: server.Transport,
	}, nil
}

type codexClientAdapter struct {
	commandAdapter commandClientAdapter
	fileAdapter    fileClientAdapter
}

func (a codexClientAdapter) id() string                       { return a.commandAdapter.id() }
func (a codexClientAdapter) label() string                    { return a.commandAdapter.label() }
func (a codexClientAdapter) capabilities() clientCapabilities { return a.commandAdapter.capabilities() }
func (a codexClientAdapter) detect() clientDetection {
	commandDetection := a.commandAdapter.detect()
	if commandDetection.Usable {
		return commandDetection
	}

	filePath := expandPath(a.fileAdapter.path)
	parentDir := filepath.Dir(filePath)
	if dirExists(parentDir) || dirExists(filepath.Dir(parentDir)) || strings.TrimSpace(filePath) != "" {
		return clientDetection{
			Installed: fileExists(filePath),
			Usable:    true,
			Location:  filePath,
		}
	}

	return clientDetection{Location: filePath}
}
func (a codexClientAdapter) buildInstallPlan(server remoteServerDefinition) (clientInstallPlan, error) {
	if a.commandAdapter.detect().Usable {
		return a.commandAdapter.buildInstallPlan(server)
	}
	return a.fileAdapter.buildInstallPlan(server)
}

type fileFieldNames struct {
	Type    string
	URL     string
	Headers string
}

type fileClientAdapter struct {
	clientID           string
	clientLabel        string
	format             string
	path               string
	topLevelKey        string
	fieldNames         fileFieldNames
	typeValue          map[installTransport]string
	includeTypeField   bool
	clientCapabilities clientCapabilities
}

func (a fileClientAdapter) id() string                       { return a.clientID }
func (a fileClientAdapter) label() string                    { return a.clientLabel }
func (a fileClientAdapter) capabilities() clientCapabilities { return a.clientCapabilities }
func (a fileClientAdapter) detect() clientDetection {
	path := expandPath(a.path)
	if fileExists(path) || dirExists(filepath.Dir(path)) {
		return clientDetection{Installed: true, Usable: true, Location: path}
	}
	return clientDetection{Installed: false, Usable: false, Location: path}
}
func (a fileClientAdapter) buildInstallPlan(server remoteServerDefinition) (clientInstallPlan, error) {
	containerKey := firstNonEmpty(a.topLevelKey, "mcpServers")
	typeField := firstNonEmpty(a.fieldNames.Type, "type")
	urlField := firstNonEmpty(a.fieldNames.URL, "url")
	headersField := firstNonEmpty(a.fieldNames.Headers, "headers")
	includeTypeField := true
	if !a.includeTypeField {
		includeTypeField = false
	}

	entry := map[string]any{
		urlField: server.URL,
	}
	if includeTypeField {
		typeValue := a.typeValue[server.Transport]
		if strings.TrimSpace(typeValue) == "" {
			typeValue = transportTypeValue(server.Transport)
		}
		entry[typeField] = typeValue
	}

	return fileInstallPlan{
		Method:    installPlanMethodFile,
		Format:    a.format,
		Path:      expandPath(a.path),
		Content:   map[string]any{containerKey: map[string]any{server.Name: entry}, "_headers_field": headersField},
		Transport: server.Transport,
	}, nil
}

type integrationClientRow struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Method    string `json:"method"`
	Transport string `json:"transport"`
	Installed bool   `json:"installed"`
	Usable    bool   `json:"usable"`
	Location  string `json:"location,omitempty"`
}

type integrationInstallResult struct {
	Client      string              `json:"client,omitempty"`
	Method      string              `json:"method,omitempty"`
	Transport   string              `json:"transport"`
	Integration string              `json:"integration"`
	EndpointURL string              `json:"endpoint_url"`
	TokenID     string              `json:"token_id"`
	Token       string              `json:"token,omitempty"`
	Command     *commandInstallPlan `json:"command,omitempty"`
	File        *fileInstallPlan    `json:"file,omitempty"`
}

var integrationClientAdapters = map[string]clientAdapter{}

func init() {
	paths := currentPlatformPaths()

	addAdapter := func(adapter clientAdapter) {
		integrationClientAdapters[adapter.id()] = adapter
	}

	streamableOnly := clientCapabilities{Transports: []installTransport{installTransportStreamableHTTP}}

	addAdapter(commandClientAdapter{
		clientID: "claude-code", clientLabel: "Claude Code", command: "claude", template: claudeCodeRemoteCommand, clientCapabilities: streamableOnly,
	})
	addAdapter(commandClientAdapter{
		clientID: "vscode", clientLabel: "VS Code", command: commandForPlatform("code", "code.cmd"), template: vscodeRemoteCommand, clientCapabilities: streamableOnly,
	})
	addAdapter(commandClientAdapter{
		clientID: "vscode-insiders", clientLabel: "VS Code Insiders", command: commandForPlatform("code-insiders", "code-insiders.cmd"), template: vscodeRemoteCommand, clientCapabilities: streamableOnly,
	})
	addAdapter(commandClientAdapter{
		clientID: "gemini-cli", clientLabel: "Gemini CLI", command: "gemini", template: geminiRemoteCommand, clientCapabilities: streamableOnly,
	})

	newFileAdapter := func(id, label, format, path string) fileClientAdapter {
		return fileClientAdapter{
			clientID: id, clientLabel: label, format: format, path: path,
			topLevelKey: "mcpServers",
			fieldNames:  fileFieldNames{Type: "type", URL: "url", Headers: "headers"},
			typeValue: map[installTransport]string{
				installTransportStreamableHTTP: "http",
				installTransportSSE:            "sse",
			},
			includeTypeField:   true,
			clientCapabilities: streamableOnly,
		}
	}

	addAdapter(newFileAdapter("cursor", "Cursor", "json", filepath.Join(paths.homeDir, ".cursor", "mcp.json")))
	addAdapter(newFileAdapter("claude", "Claude Desktop", "json", filepath.Join(paths.baseDir, "Claude", "claude_desktop_config.json")))
	addAdapter(newFileAdapter("witsy", "Witsy", "json", filepath.Join(paths.baseDir, "Witsy", "settings.json")))
	addAdapter(newFileAdapter("enconvo", "Enconvo", "json", filepath.Join(paths.homeDir, ".config", "enconvo", "mcp_config.json")))
	addAdapter(newFileAdapter("roocode", "Roo Code", "json", filepath.Join(paths.baseDir, paths.vscodePath, "rooveterinaryinc.roo-cline", "settings", "mcp_settings.json")))
	addAdapter(newFileAdapter("bolttai", "BoltAI", "json", filepath.Join(paths.homeDir, ".boltai", "mcp.json")))
	addAdapter(newFileAdapter("amazon-bedrock", "Amazon Bedrock", "json", filepath.Join(paths.homeDir, "Amazon Bedrock Client", "mcp_config.json")))
	addAdapter(newFileAdapter("amazonq", "Amazon Q", "json", filepath.Join(paths.homeDir, ".aws", "amazonq", "mcp.json")))
	addAdapter(newFileAdapter("tome", "Tome", "json", filepath.Join(paths.homeDir, ".tome", "mcp_config.json")))
	addAdapter(newFileAdapter("librechat", "LibreChat", "yaml", filepath.Join(firstNonEmpty(os.Getenv("LIBRECHAT_CONFIG_DIR"), paths.homeDir), "LibreChat", "librechat.yaml")))

	windsurf := newFileAdapter("windsurf", "Windsurf", "json", filepath.Join(paths.homeDir, ".codeium", "windsurf", "mcp_config.json"))
	windsurf.fieldNames.URL = "serverUrl"
	addAdapter(windsurf)

	cline := newFileAdapter("cline", "Cline", "json", filepath.Join(paths.baseDir, paths.vscodePath, "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json"))
	cline.typeValue[installTransportStreamableHTTP] = "streamableHttp"
	addAdapter(cline)

	opencode := newFileAdapter("opencode", "OpenCode", "jsonc", filepath.Join(paths.homeDir, ".opencode", "opencode.jsonc"))
	opencode.topLevelKey = "mcp"
	opencode.typeValue[installTransportStreamableHTTP] = "remote"
	opencode.typeValue[installTransportSSE] = "remote"
	addAdapter(opencode)

	goose := newFileAdapter("goose", "Goose", "yaml", filepath.Join(paths.homeDir, ".config", "goose", "config.yaml"))
	goose.topLevelKey = "extensions"
	addAdapter(goose)

	codexConfig := newFileAdapter("codex-config", "Codex (config.toml)", "toml", filepath.Join(paths.homeDir, ".codex", "config.toml"))
	codexConfig.topLevelKey = "mcp_servers"
	codexConfig.includeTypeField = false
	addAdapter(codexClientAdapter{
		commandAdapter: commandClientAdapter{
			clientID: "codex", clientLabel: "Codex", command: "codex", template: codexRemoteCommand, clientCapabilities: streamableOnly,
		},
		fileAdapter: codexConfig,
	})
	addAdapter(codexConfig)
}

type platformClientPaths struct {
	homeDir    string
	baseDir    string
	vscodePath string
}

func currentPlatformPaths() platformClientPaths {
	homeDir, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return platformClientPaths{
			homeDir:    homeDir,
			baseDir:    firstNonEmpty(os.Getenv("APPDATA"), filepath.Join(homeDir, "AppData", "Roaming")),
			vscodePath: filepath.Join("Code", "User", "globalStorage"),
		}
	case "darwin":
		return platformClientPaths{
			homeDir:    homeDir,
			baseDir:    filepath.Join(homeDir, "Library", "Application Support"),
			vscodePath: filepath.Join("Code", "User", "globalStorage"),
		}
	default:
		return platformClientPaths{
			homeDir:    homeDir,
			baseDir:    firstNonEmpty(os.Getenv("XDG_CONFIG_HOME"), filepath.Join(homeDir, ".config")),
			vscodePath: filepath.Join("Code", "User", "globalStorage"),
		}
	}
}

func commandForPlatform(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return unix
}

func claudeCodeRemoteCommand(input remoteCommandInput) []string {
	return []string{"mcp", "add", "--transport", commandTransportValue(input.Transport), input.Name, input.URL}
}

func vscodeRemoteCommand(input remoteCommandInput) []string {
	payload := map[string]any{
		"name": input.Name,
		"type": commandTransportValue(input.Transport),
		"url":  input.URL,
	}
	encoded, _ := json.Marshal(payload)
	return []string{"--add-mcp", string(encoded)}
}

func geminiRemoteCommand(input remoteCommandInput) []string {
	return []string{"mcp", "add", "--transport", commandTransportValue(input.Transport), input.Name, input.URL}
}

func codexRemoteCommand(input remoteCommandInput) []string {
	args := []string{"mcp", "add", input.Name, "--url", input.URL}
	if input.Transport == installTransportSSE {
		args = append(args, "--transport", "sse")
	}
	return args
}

func commandTransportValue(transport installTransport) string {
	if transport == installTransportSSE {
		return "sse"
	}
	return "http"
}

func transportTypeValue(transport installTransport) string {
	if transport == installTransportSSE {
		return "sse"
	}
	return "http"
}

func integrationClientByID(id string) (clientAdapter, error) {
	adapter, ok := integrationClientAdapters[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		ids := make([]string, 0, len(integrationClientAdapters))
		for key := range integrationClientAdapters {
			ids = append(ids, key)
		}
		return nil, fmt.Errorf("metorial: unknown client %q. Available clients: %s", id, strings.Join(ids, ", "))
	}
	return adapter, nil
}

func integrationClientRows() []integrationClientRow {
	rows := make([]integrationClientRow, 0, len(integrationClientAdapters))
	for _, adapter := range integrationClientAdapters {
		detection := adapter.detect()
		rows = append(rows, integrationClientRow{
			ID:        adapter.id(),
			Label:     adapter.label(),
			Method:    string(methodForAdapter(adapter)),
			Transport: string(selectInstallTransport(adapter.capabilities())),
			Installed: detection.Installed,
			Usable:    detection.Usable,
			Location:  detection.Location,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func methodForAdapter(adapter clientAdapter) installPlanMethod {
	switch adapter.(type) {
	case commandClientAdapter:
		return installPlanMethodCommand
	case codexClientAdapter:
		return installPlanMethodCommand
	case fileClientAdapter:
		return installPlanMethodFile
	default:
		return ""
	}
}

func selectInstallTransport(capabilities clientCapabilities) installTransport {
	for _, transport := range capabilities.Transports {
		if transport == installTransportStreamableHTTP {
			return installTransportStreamableHTTP
		}
	}
	for _, transport := range capabilities.Transports {
		if transport == installTransportSSE {
			return installTransportSSE
		}
	}
	return installTransportStreamableHTTP
}

func createMagicMcpInstallToken(sdk *metorial.MetorialSdk, magicMcpServerID string) (*magicmcptokens.MagicMcpTokensCreateOutput, error) {
	name := "Metorial CLI"
	return sdk.MagicMcpTokens.Create(&endpoints.MagicMcpTokensEndpointCreateBody{
		Name:             name,
		MagicMcpServerId: &magicMcpServerID,
	})
}

func buildMagicMcpInstallURL(server *magicmcpservers.MagicMcpServersGetOutput, tokenSecret string) (string, error) {
	if server == nil {
		return "", fmt.Errorf("metorial: integration is required")
	}
	endpointURL := primaryServerURLFromGet(server.Endpoints)
	if strings.TrimSpace(endpointURL) == "" {
		return "", fmt.Errorf("metorial: integration %s has no MCP endpoint", integrationGetIdentifier(server))
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return "", fmt.Errorf("metorial: invalid integration endpoint %q: %w", endpointURL, err)
	}
	query := parsed.Query()
	query.Set("key", tokenSecret)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func executeIntegrationInstallPlan(plan clientInstallPlan) error {
	switch typed := plan.(type) {
	case commandInstallPlan:
		command := exec.Command(typed.Command, typed.Args...)
		output, err := command.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message != "" {
				return fmt.Errorf("metorial: failed to run %s %s: %s", typed.Command, strings.Join(typed.Args, " "), message)
			}
			return fmt.Errorf("metorial: failed to run %s %s: %w", typed.Command, strings.Join(typed.Args, " "), err)
		}
		return nil
	case fileInstallPlan:
		return writeIntegrationInstallFile(typed)
	default:
		return fmt.Errorf("metorial: unsupported install plan")
	}
}

func writeIntegrationInstallFile(plan fileInstallPlan) error {
	path := expandPath(plan.Path)
	existing, err := readInstallDocument(path, plan.Format)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}

	headersField, _ := plan.Content["_headers_field"].(string)
	delete(plan.Content, "_headers_field")

	for key, value := range plan.Content {
		container, ok := value.(map[string]any)
		if !ok {
			existing[key] = value
			continue
		}

		current, _ := existing[key].(map[string]any)
		if current == nil {
			current = map[string]any{}
		}
		for serverName, entryValue := range container {
			entry, _ := entryValue.(map[string]any)
			if entry == nil {
				current[serverName] = entryValue
				continue
			}

			prev, _ := current[serverName].(map[string]any)
			if prev == nil {
				prev = map[string]any{}
			}
			for entryKey, entryItem := range entry {
				prev[entryKey] = entryItem
			}
			if headersField != "" {
				if _, exists := prev[headersField]; !exists {
					delete(prev, headersField)
				}
			}
			current[serverName] = prev
		}
		existing[key] = current
	}

	encoded, err := encodeInstallDocument(existing, plan.Format)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("metorial: failed to create config directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("metorial: failed to write %s: %w", path, err)
	}
	return nil
}

func readInstallDocument(path string, format string) (map[string]any, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("metorial: failed to read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return map[string]any{}, nil
	}

	var value map[string]any
	switch format {
	case "json":
		err = json.Unmarshal(payload, &value)
	case "jsonc":
		standardized, standardizeErr := hujson.Standardize(payload)
		if standardizeErr != nil {
			return nil, fmt.Errorf("metorial: failed to parse JSONC config %s: %w", path, standardizeErr)
		}
		err = json.Unmarshal(standardized, &value)
	case "yaml":
		err = yaml.Unmarshal(payload, &value)
	case "toml":
		err = toml.Unmarshal(payload, &value)
	default:
		return nil, fmt.Errorf("metorial: unsupported config format %q", format)
	}
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to parse %s: %w", path, err)
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}

func encodeInstallDocument(value map[string]any, format string) ([]byte, error) {
	switch format {
	case "json", "jsonc":
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to encode JSON config: %w", err)
		}
		return append(encoded, '\n'), nil
	case "yaml":
		encoded, err := yaml.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to encode YAML config: %w", err)
		}
		return encoded, nil
	case "toml":
		encoded, err := toml.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to encode TOML config: %w", err)
		}
		return encoded, nil
	default:
		return nil, fmt.Errorf("metorial: unsupported config format %q", format)
	}
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~"+string(os.PathSeparator)))
	}
	return path
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func renderIntegrationClientList(writer io.Writer, features terminal.Features, rows []integrationClientRow, tips []string) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Bold("Supported Clients"))
	_, _ = fmt.Fprintln(writer)

	table := output.Table{
		Columns:  []string{"Client", "Method", "Transport", "Installed", "Usable"},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, row := range rows {
		table.Rows = append(table.Rows, []string{
			colors.Bold(row.Label) + "\n" + colors.Muted(row.ID) + "\n",
			row.Method,
			row.Transport,
			boolWord(row.Installed),
			boolWord(row.Usable),
		})
	}
	if err := table.Render(writer); err != nil {
		return err
	}
	return renderTips(writer, features, tips)
}

func renderIntegrationInstallResult(writer io.Writer, features terminal.Features, result integrationInstallResult) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Success("Integration Installed"))
	_, _ = fmt.Fprintln(writer)

	items := []output.DataListItem{
		{Label: "Integration", Value: result.Integration},
		{Label: "Client", Value: result.Client},
		{Label: "Method", Value: result.Method},
		{Label: "Transport", Value: result.Transport},
		{Label: "Token", Value: result.TokenID},
	}
	if result.File != nil {
		items = append(items, output.DataListItem{Label: "Config File", Value: result.File.Path})
	}
	if result.Command != nil {
		items = append(items, output.DataListItem{Label: "Command", Value: result.Command.Command + " " + strings.Join(result.Command.Args, " ")})
	}
	if err := output.RenderDataList(writer, items); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(writer)
	return output.RenderBox(writer, []string{
		colors.Accent("Endpoint URL"),
		previewInstallURLKey(result.EndpointURL, 20),
	}, output.BoxOptions{MaxWidth: features.Width, Unicode: features.HasUnicode})
}

func renderCustomInstallResult(writer io.Writer, features terminal.Features, result integrationInstallResult) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Success("Custom MCP Token Created"))
	_, _ = fmt.Fprintln(writer)
	if err := output.RenderDataList(writer, []output.DataListItem{
		{Label: "Integration", Value: result.Integration},
		{Label: "Transport", Value: result.Transport},
		{Label: "Token ID", Value: result.TokenID},
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(writer)
	return output.RenderBox(writer, []string{
		colors.Accent("Endpoint URL"),
		colors.Bold(result.EndpointURL),
		"",
		colors.Accent("Token"),
		colors.Bold(result.Token),
	}, output.BoxOptions{MaxWidth: features.Width, Unicode: features.HasUnicode})
}

func boolWord(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func previewInstallURLKey(rawURL string, visible int) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsed.Query()
	key := query.Get("key")
	if strings.TrimSpace(key) == "" {
		return rawURL
	}
	if visible < 0 {
		visible = 0
	}
	if len(key) > visible {
		key = key[:visible] + "..."
	}
	query.Set("key", key)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
