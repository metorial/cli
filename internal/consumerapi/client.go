package consumerapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/metorial/cli/internal/apiclient"
	"github.com/metorial/cli/internal/config"
	metorial "github.com/metorial/metorial-go/v1"
)

type Client struct {
	runtime config.Runtime
}

func New(runtime config.Runtime) (*Client, error) {
	if err := runtime.RequireAPIKey(); err != nil {
		return nil, err
	}

	return &Client{runtime: runtime}, nil
}

func (c *Client) AccessToken() string {
	return strings.TrimSpace(c.runtime.APIKey)
}

func (c *Client) GetProfile() (*Profile, error) {
	result := &Profile{}
	return result, c.do(http.MethodPost, "/consumer/profile", nil, result)
}

func (c *Client) ListProviderGroups(options ListOptions) (*ProviderGroupList, error) {
	result := &ProviderGroupList{}
	return result, c.do(http.MethodGet, "/consumer/providers/groups"+apiclient.ListQuery(options, nil), nil, result)
}

func (c *Client) ListProviders(options ListOptions, search string, providerGroupID string) (*ProviderList, error) {
	extra := url.Values{}
	if search = strings.TrimSpace(search); search != "" {
		extra.Set("search", search)
	}
	if providerGroupID = strings.TrimSpace(providerGroupID); providerGroupID != "" {
		extra.Set("provider_group_id", providerGroupID)
	}

	result := &ProviderList{}
	return result, c.do(http.MethodGet, "/consumer/providers"+apiclient.ListQuery(options, extra), nil, result)
}

func (c *Client) GetProvider(catalogItemID string) (*Provider, error) {
	result := &Provider{}
	return result, c.do(http.MethodGet, "/consumer/providers/"+apiclient.PathSegment(catalogItemID), nil, result)
}

func (c *Client) RequestAccess(catalogItemID string, message string) (*AccessRequest, error) {
	body := map[string]any{}
	if message = strings.TrimSpace(message); message != "" {
		body["message"] = message
	}

	result := &AccessRequest{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+apiclient.PathSegment(catalogItemID)+"/request-access", body, result)
}

func (c *Client) StartSetup(catalogItemID string, authMethodID string) (*SetupSession, error) {
	body := map[string]any{}
	if authMethodID = strings.TrimSpace(authMethodID); authMethodID != "" {
		body["provider_auth_method_id"] = authMethodID
	}

	result := &SetupSession{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+apiclient.PathSegment(catalogItemID)+"/setup", body, result)
}

func (c *Client) GetSetup(catalogItemID string, setupSessionID string) (*SetupSession, error) {
	result := &SetupSession{}
	target := "/consumer/providers/" + apiclient.PathSegment(catalogItemID) + "/setup/" + apiclient.PathSegment(setupSessionID)
	return result, c.do(http.MethodGet, target, nil, result)
}

func (c *Client) Deploy(catalogItemID string, input DeployInput) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+apiclient.PathSegment(catalogItemID)+"/deploy", input, result)
}

func (c *Client) Reconnect(catalogItemID string, magicMcpServerID string, setupSessionID string) (*MagicMcpServer, error) {
	body := map[string]any{
		"magic_mcp_server_id":          magicMcpServerID,
		"integration_setup_session_id": setupSessionID,
	}

	result := &MagicMcpServer{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+apiclient.PathSegment(catalogItemID)+"/reconnect", body, result)
}

func (c *Client) GetProviderSpecification(specificationID string) (*ProviderSpecification, error) {
	result := &ProviderSpecification{}
	return result, c.do(http.MethodGet, "/provider-specifications/"+apiclient.PathSegment(specificationID), nil, result)
}

func (c *Client) ListMagicMcpServers(options ListOptions, search string, providerTemplateID string) (*MagicMcpServerList, error) {
	extra := url.Values{}
	extra.Set("status", "active")
	if search = strings.TrimSpace(search); search != "" {
		extra.Set("search", search)
	}
	if providerTemplateID = strings.TrimSpace(providerTemplateID); providerTemplateID != "" {
		extra.Set("provider_template_id", providerTemplateID)
	}

	result := &MagicMcpServerList{}
	return result, c.do(http.MethodGet, "/magic-mcp-servers"+apiclient.ListQuery(options, extra), nil, result)
}

func (c *Client) GetMagicMcpServer(magicMcpServerID string) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodGet, "/magic-mcp-servers/"+apiclient.PathSegment(magicMcpServerID), nil, result)
}

func (c *Client) UpdateMagicMcpServer(magicMcpServerID string, input UpdateMagicMcpServerInput) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodPatch, "/magic-mcp-servers/"+apiclient.PathSegment(magicMcpServerID), input, result)
}

func (c *Client) DeleteMagicMcpServer(magicMcpServerID string) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodDelete, "/magic-mcp-servers/"+apiclient.PathSegment(magicMcpServerID), nil, result)
}

func (c *Client) ListMagicMcpTokens(options ListOptions) (*MagicMcpTokenList, error) {
	extra := url.Values{}
	extra.Set("status", "active")

	result := &MagicMcpTokenList{}
	return result, c.do(http.MethodGet, "/magic-mcp-tokens"+apiclient.ListQuery(options, extra), nil, result)
}

func (c *Client) CreateMagicMcpToken(input CreateMagicMcpTokenInput) (*MagicMcpToken, error) {
	result := &MagicMcpToken{}
	return result, c.do(http.MethodPost, "/magic-mcp-tokens", input, result)
}

func (c *Client) DeleteMagicMcpToken(magicMcpTokenID string) (*MagicMcpToken, error) {
	result := &MagicMcpToken{}
	return result, c.do(http.MethodDelete, "/magic-mcp-tokens/"+apiclient.PathSegment(magicMcpTokenID), nil, result)
}

func (c *Client) do(method string, target string, body any, output any) error {
	return apiclient.Client{Runtime: c.runtime, Describe: describeError}.Do(method, target, body, output)
}

func describeError(target string, err error) error {
	apiErr := &metorial.Error{}
	if !errors.As(err, &apiErr) {
		return err
	}

	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("metorial: your portal session is no longer valid: %s\nRun \"metorial login\" to sign in again", apiErr.Message)
	case http.StatusPaymentRequired:
		return fmt.Errorf("metorial: portals are not available on this organization's plan: %s", apiErr.Message)
	case http.StatusForbidden:
		if strings.Contains(strings.ToLower(apiErr.Message), "not entitled") {
			return fmt.Errorf("metorial: portals or magic MCP are not enabled for this organization (%s)", target)
		}
	}

	return err
}
