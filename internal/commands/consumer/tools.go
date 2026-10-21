package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newToolCommands(env commandEnv) []*cobra.Command {
	return []*cobra.Command{
		newToolsCommand(env),
		newSchemaCommand(env),
		newCallCommand(env),
	}
}

func newToolsCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "tools <integration-id>",
		Short: "View the providers and tools of an integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			server, mcpClient, err := connectIntegration(commandContext(command), env, args[0])
			if err != nil {
				return err
			}
			defer mcpClient.Close()

			liveTools, err := mcpClient.ListTools(commandContext(command))
			if err != nil {
				return err
			}

			tips := []string{}
			if len(liveTools) > 0 {
				identifier := integrationIdentifier(server)
				toolKey := liveTools[0].Name
				tips = append(tips,
					fmt.Sprintf("metorial schema %s %s", identifier, toolKey),
					fmt.Sprintf("metorial call %s %s --data '{}'", identifier, toolKey),
				)
				tips = append(tips, integrationTips(server, true)[1:]...)
			}

			return env.ctx.Write(command, map[string]any{
				"integration": server,
				"providers":   server.Providers,
				"tools":       liveTools,
				"tips":        tips,
			}, func(writer io.Writer, features terminal.Features) error {
				return renderIntegrationTools(writer, features, server, liveTools, tips)
			})
		},
	}
}

func newSchemaCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "schema <integration-id> <tool-key>",
		Short: "View the input schema for an integration tool",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			_, mcpClient, err := connectIntegration(commandContext(command), env, args[0])
			if err != nil {
				return err
			}
			defer mcpClient.Close()

			tool, err := mcpClient.FindToolByName(commandContext(command), args[1])
			if err != nil {
				return err
			}

			schemaValue := tool.InputSchema
			if schemaValue == nil {
				schemaValue = map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": true,
				}
			}

			return writeIntegrationMachineValue(command.OutOrStdout(), env.app().StdoutFeatures(), env.format(), schemaValue)
		},
	}
}

func newCallCommand(env commandEnv) *cobra.Command {
	data := ""

	command := &cobra.Command{
		Use:   "call <integration-id> <tool-key>",
		Short: "Call an integration tool over MCP",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			ctx := commandContext(command)
			server, mcpClient, err := connectIntegration(ctx, env, args[0])
			if err != nil {
				return err
			}
			defer mcpClient.Close()

			tool, err := mcpClient.FindToolByName(ctx, args[1])
			if err != nil {
				return err
			}

			input, preview, err := resolveIntegrationToolInput(env.app(), data)
			if err != nil {
				return err
			}
			if err := validateIntegrationToolInput(tool, input, preview, env.app().StderrFeatures()); err != nil {
				return err
			}

			result, err := mcpClient.CallTool(ctx, tool.Name, input)
			if err != nil {
				return err
			}
			if result.IsError {
				return formatMCPToolResultError(env.app().StderrFeatures(), server, tool, result)
			}

			return writeIntegrationMachineValue(command.OutOrStdout(), env.app().StdoutFeatures(), env.format(), normalizeMCPCallResult(result))
		},
	}
	command.Flags().StringVarP(&data, "data", "d", "", "JSON tool input, or @- to read from stdin")
	return command
}

func commandContext(command *cobra.Command) context.Context {
	if ctx := command.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func connectIntegration(ctx context.Context, env commandEnv, integrationID string) (*consumerapi.MagicMcpServer, *magicMCPClient, error) {
	client, err := env.client()
	if err != nil {
		return nil, nil, err
	}

	server, err := client.GetMagicMcpServer(integrationID)
	if err != nil {
		return nil, nil, err
	}

	mcpClient, err := connectMagicMCP(ctx, client.AccessToken(), server)
	if err != nil {
		return nil, nil, err
	}

	return server, mcpClient, nil
}

func renderIntegrationTools(writer io.Writer, features terminal.Features, server *consumerapi.MagicMcpServer, tools []*mcp.Tool, tips []string) error {
	colors := terminal.NewColorizer(features)
	serverTitle := integrationTitle(server)
	commandutil.RenderTitle(writer, features, serverTitle, server.ID)

	for _, provider := range server.Providers {
		if provider.Provider == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(provider.Provider.Name), strings.TrimSpace(serverTitle)) {
			_, _ = fmt.Fprintln(writer, colors.Accent(provider.Provider.Name))
		}
		_, _ = fmt.Fprintln(writer, colors.Muted(provider.Provider.Slug))
		if description := commandutil.OptionalString(provider.Provider.Description); description != "" {
			_, _ = fmt.Fprintln(writer, description)
		}
	}

	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Accent("Tools"))
	if len(tools) == 0 {
		_, _ = fmt.Fprintln(writer, "(no tools)")
		return commandutil.RenderTips(writer, features, tips)
	}

	table := output.Table{
		Columns:  []string{colors.Accent("Name"), colors.Accent("Title"), colors.Accent("Description")},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		table.Rows = append(table.Rows, []string{tool.Name, commandutil.FirstNonEmpty(tool.Title, tool.Name), tool.Description})
	}
	if err := table.Render(writer); err != nil {
		return err
	}

	return commandutil.RenderTips(writer, features, tips)
}

func resolveIntegrationToolInput(application *app.App, data string) (map[string]any, string, error) {
	raw, err := readIntegrationToolInput(application, data)
	if err != nil {
		return nil, "", err
	}

	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, "{}", nil
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, "", fmt.Errorf("%s", formatIntegrationInputParseError(application.StderrFeatures(), raw, err))
	}

	object, ok := value.(map[string]any)
	if !ok {
		preview := previewJSON(value)
		return nil, preview, fmt.Errorf("%s", formatIntegrationInputObjectError(application.StderrFeatures(), preview))
	}

	return object, previewJSON(object), nil
}

func readIntegrationToolInput(application *app.App, data string) (string, error) {
	switch strings.TrimSpace(data) {
	case "", "@-", "-":
	default:
		return data, nil
	}

	if strings.TrimSpace(data) == "@-" || strings.TrimSpace(data) == "-" {
		payload, err := io.ReadAll(application.Stdin)
		if err != nil {
			return "", fmt.Errorf("metorial: failed to read tool input from stdin: %w", err)
		}
		return string(payload), nil
	}

	if file, ok := application.Stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return "{}", nil
	}

	payload, err := io.ReadAll(application.Stdin)
	if err != nil {
		return "", fmt.Errorf("metorial: failed to read tool input from stdin: %w", err)
	}
	if strings.TrimSpace(string(payload)) == "" {
		return "{}", nil
	}

	return string(payload), nil
}

func validateIntegrationToolInput(tool *mcp.Tool, input map[string]any, preview string, features terminal.Features) error {
	schemaValue := map[string]any{
		"type": "object",
	}
	if tool != nil && tool.InputSchema != nil {
		typed, err := normalizeSchemaObject(tool.InputSchema)
		if err != nil {
			return fmt.Errorf("metorial: failed to parse MCP input schema for tool %q: %w", tool.Name, err)
		}
		schemaValue = typed
	}

	schemaBytes, err := json.Marshal(schemaValue)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode input schema for tool %q: %w", tool.Name, err)
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("metorial: failed to decode input schema for tool %q: %w", tool.Name, err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("metorial: failed to resolve input schema for tool %q: %w", tool.Name, err)
	}

	if err := resolved.Validate(input); err != nil {
		return fmt.Errorf("%s", formatIntegrationInputValidationError(features, tool, preview, err))
	}

	return nil
}

func normalizeSchemaObject(value any) (map[string]any, error) {
	if typed, ok := value.(map[string]any); ok {
		return typed, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}

	return decoded, nil
}

func previewJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}

	text := string(encoded)
	if len(text) > 600 {
		return text[:600] + "\n..."
	}
	return text
}

func formatIntegrationInputValidationError(features terminal.Features, tool *mcp.Tool, preview string, validationErr error) string {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Input validation failed"),
		fmt.Sprintf("Tool: %s", colors.Bold(commandutil.FirstNonEmpty(tool.Title, tool.Name))),
		fmt.Sprintf("Key: %s", tool.Name),
		"",
		colors.Accent("Why"),
		validationErr.Error(),
		"",
		colors.Accent("Input Preview"),
		preview,
	}

	return strings.Join(lines, "\n")
}

func formatIntegrationInputParseError(features terminal.Features, raw string, parseErr error) string {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Input parsing failed"),
		"Tool input must be valid JSON.",
		"",
		colors.Accent("Why"),
		parseErr.Error(),
		"",
		colors.Accent("Input Preview"),
		previewJSON(strings.TrimSpace(raw)),
	}

	return strings.Join(lines, "\n")
}

func formatIntegrationInputObjectError(features terminal.Features, preview string) string {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Input shape is invalid"),
		"Tool input must be a JSON object at the top level.",
		"",
		colors.Accent("Input Preview"),
		preview,
	}

	return strings.Join(lines, "\n")
}

func normalizeMCPCallResult(result *mcp.CallToolResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}

	if result.StructuredContent != nil {
		if typed, ok := result.StructuredContent.(map[string]any); ok {
			return typed
		}

		return map[string]any{
			"value": result.StructuredContent,
		}
	}

	contentValues := mcpContentToValues(result.Content)
	if len(contentValues) == 1 {
		if typed, ok := contentValues[0].(map[string]any); ok {
			if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
				var decoded any
				if json.Unmarshal([]byte(text), &decoded) == nil {
					if object, ok := decoded.(map[string]any); ok {
						return object
					}
					return map[string]any{"value": decoded}
				}
				return map[string]any{"text": text}
			}
			return typed
		}
		if text, ok := contentValues[0].(string); ok {
			return map[string]any{"text": text}
		}
	}

	return map[string]any{
		"content": contentValues,
	}
}

func formatMCPToolResultError(features terminal.Features, server *consumerapi.MagicMcpServer, tool *mcp.Tool, result *mcp.CallToolResult) error {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Tool call failed"),
		fmt.Sprintf("Integration: %s", colors.Bold(integrationTitle(server))),
		fmt.Sprintf("Tool: %s", colors.Bold(commandutil.FirstNonEmpty(tool.Title, tool.Name))),
	}

	message := ""
	for _, item := range mcpContentToValues(result.Content) {
		switch typed := item.(type) {
		case map[string]any:
			if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
				message = text
				break
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				message = typed
				break
			}
		}
	}

	if strings.TrimSpace(message) != "" {
		lines = append(lines, "", colors.Accent("Server Message"), message)
	}

	if result.StructuredContent != nil {
		lines = append(lines, "", colors.Accent("Structured Error"), previewJSON(result.StructuredContent))
	}

	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

func writeIntegrationMachineValue(writer io.Writer, features terminal.Features, formatInput string, value any) error {
	format, err := output.ParseFormat(formatInput)
	if err != nil {
		return err
	}
	if format == output.FormatStructured {
		format = output.FormatJSON
	}

	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode response: %w", err)
	}

	return output.WriteResponse(writer, &fetch.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       body,
	}, output.RenderOptions{
		Format: format,
		Colors: features,
	})
}
