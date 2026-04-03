package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/metorial/cli/internal/version"
	"github.com/metorial/metorial-go/v1/resources/magicmcpservers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type magicMCPClient struct {
	session *mcp.ClientSession
}

func connectMagicMCP(ctx context.Context, tokenSecret string, consumerProfileID string, server *magicmcpservers.MagicMcpServersGetOutput) (*magicMCPClient, error) {
	if server == nil {
		return nil, fmt.Errorf("metorial: integration is required")
	}
	if strings.TrimSpace(tokenSecret) == "" {
		return nil, fmt.Errorf("metorial: magic MCP token secret is required")
	}

	endpointURL := primaryServerURLFromGet(server.Endpoints)
	if strings.TrimSpace(endpointURL) == "" {
		return nil, fmt.Errorf("metorial: integration %s has no MCP endpoint", integrationGetIdentifier(server))
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "metorial-cli",
		Version: version.Version,
	}, nil)

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	session, err := client.Connect(connectCtx, &mcp.StreamableClientTransport{
		Endpoint:             endpointURL,
		DisableStandaloneSSE: true,
		HTTPClient: &http.Client{
			Transport: &magicMCPAuthTransport{
				base:              http.DefaultTransport,
				bearerToken:       tokenSecret,
				consumerProfileID: consumerProfileID,
			},
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to connect to MCP endpoint %s: %w", endpointURL, err)
	}

	return &magicMCPClient{session: session}, nil
}

func (c *magicMCPClient) Close() error {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.Close()
}

func (c *magicMCPClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("metorial: MCP session is not connected")
	}

	var all []*mcp.Tool
	var cursor string

	for {
		listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := c.session.ListTools(listCtx, &mcp.ListToolsParams{
			Cursor: cursor,
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to list tools from MCP endpoint: %w", err)
		}

		all = append(all, result.Tools...)
		if strings.TrimSpace(result.NextCursor) == "" {
			break
		}
		cursor = result.NextCursor
	}

	return all, nil
}

func (c *magicMCPClient) FindToolByName(ctx context.Context, toolName string) (*mcp.Tool, error) {
	tools, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	needle := strings.TrimSpace(toolName)
	for _, tool := range tools {
		if tool != nil && strings.EqualFold(strings.TrimSpace(tool.Name), needle) {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("metorial: tool %q was not found on this integration", toolName)
}

func (c *magicMCPClient) CallTool(ctx context.Context, toolName string, input map[string]any) (*mcp.CallToolResult, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("metorial: MCP session is not connected")
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := c.session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: input,
	})
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to call tool %q: %w", toolName, err)
	}

	return result, nil
}

func mcpContentToValues(contents []mcp.Content) []any {
	values := make([]any, 0, len(contents))
	for _, content := range contents {
		if content == nil {
			continue
		}

		encoded, err := json.Marshal(content)
		if err != nil {
			values = append(values, map[string]any{"type": "unknown", "error": err.Error()})
			continue
		}

		var value any
		if err := json.Unmarshal(encoded, &value); err != nil {
			values = append(values, string(encoded))
			continue
		}

		values = append(values, value)
	}

	return values
}

func primaryServerURLFromGet(endpoints []magicmcpservers.MagicMcpServersGetOutputEndpoints) string {
	if len(endpoints) == 0 {
		return ""
	}
	return endpoints[0].Url
}

type magicMCPAuthTransport struct {
	base              http.RoundTripper
	bearerToken       string
	consumerProfileID string
}

func (t *magicMCPAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	if strings.TrimSpace(t.bearerToken) != "" && strings.TrimSpace(cloned.Header.Get("Authorization")) == "" {
		cloned.Header.Set("Authorization", "Bearer "+strings.TrimSpace(t.bearerToken))
	}
	if strings.TrimSpace(t.consumerProfileID) != "" && strings.TrimSpace(cloned.Header.Get("metorial-consumer-profile-id")) == "" {
		cloned.Header.Set("metorial-consumer-profile-id", strings.TrimSpace(t.consumerProfileID))
	}
	if strings.TrimSpace(cloned.Header.Get("User-Agent")) == "" {
		cloned.Header.Set("User-Agent", "metorial-cli/"+version.Version)
	}

	return base.RoundTrip(cloned)
}
