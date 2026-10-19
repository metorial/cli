package consumerapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
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
	return result, c.do(http.MethodGet, "/consumer/providers/groups"+listQuery(options, nil), nil, result)
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
	return result, c.do(http.MethodGet, "/consumer/providers"+listQuery(options, extra), nil, result)
}

func (c *Client) GetProvider(catalogItemID string) (*Provider, error) {
	result := &Provider{}
	return result, c.do(http.MethodGet, "/consumer/providers/"+pathSegment(catalogItemID), nil, result)
}

func (c *Client) RequestAccess(catalogItemID string, message string) (*AccessRequest, error) {
	body := map[string]any{}
	if message = strings.TrimSpace(message); message != "" {
		body["message"] = message
	}

	result := &AccessRequest{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+pathSegment(catalogItemID)+"/request-access", body, result)
}

func (c *Client) StartSetup(catalogItemID string, authMethodID string) (*SetupSession, error) {
	body := map[string]any{}
	if authMethodID = strings.TrimSpace(authMethodID); authMethodID != "" {
		body["provider_auth_method_id"] = authMethodID
	}

	result := &SetupSession{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+pathSegment(catalogItemID)+"/setup", body, result)
}

func (c *Client) GetSetup(catalogItemID string, setupSessionID string) (*SetupSession, error) {
	result := &SetupSession{}
	target := "/consumer/providers/" + pathSegment(catalogItemID) + "/setup/" + pathSegment(setupSessionID)
	return result, c.do(http.MethodGet, target, nil, result)
}

func (c *Client) Deploy(catalogItemID string, input DeployInput) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+pathSegment(catalogItemID)+"/deploy", input, result)
}

func (c *Client) Reconnect(catalogItemID string, magicMcpServerID string, setupSessionID string) (*MagicMcpServer, error) {
	body := map[string]any{
		"magic_mcp_server_id":          magicMcpServerID,
		"integration_setup_session_id": setupSessionID,
	}

	result := &MagicMcpServer{}
	return result, c.do(http.MethodPost, "/consumer/providers/"+pathSegment(catalogItemID)+"/reconnect", body, result)
}

func (c *Client) GetProviderSpecification(specificationID string) (*ProviderSpecification, error) {
	result := &ProviderSpecification{}
	return result, c.do(http.MethodGet, "/provider-specifications/"+pathSegment(specificationID), nil, result)
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
	return result, c.do(http.MethodGet, "/magic-mcp-servers"+listQuery(options, extra), nil, result)
}

func (c *Client) GetMagicMcpServer(magicMcpServerID string) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodGet, "/magic-mcp-servers/"+pathSegment(magicMcpServerID), nil, result)
}

func (c *Client) UpdateMagicMcpServer(magicMcpServerID string, input UpdateMagicMcpServerInput) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodPatch, "/magic-mcp-servers/"+pathSegment(magicMcpServerID), input, result)
}

func (c *Client) DeleteMagicMcpServer(magicMcpServerID string) (*MagicMcpServer, error) {
	result := &MagicMcpServer{}
	return result, c.do(http.MethodDelete, "/magic-mcp-servers/"+pathSegment(magicMcpServerID), nil, result)
}

func (c *Client) ListMagicMcpTokens(options ListOptions) (*MagicMcpTokenList, error) {
	extra := url.Values{}
	extra.Set("status", "active")

	result := &MagicMcpTokenList{}
	return result, c.do(http.MethodGet, "/magic-mcp-tokens"+listQuery(options, extra), nil, result)
}

func (c *Client) CreateMagicMcpToken(input CreateMagicMcpTokenInput) (*MagicMcpToken, error) {
	result := &MagicMcpToken{}
	return result, c.do(http.MethodPost, "/magic-mcp-tokens", input, result)
}

func (c *Client) DeleteMagicMcpToken(magicMcpTokenID string) (*MagicMcpToken, error) {
	result := &MagicMcpToken{}
	return result, c.do(http.MethodDelete, "/magic-mcp-tokens/"+pathSegment(magicMcpTokenID), nil, result)
}

func (c *Client) do(method string, target string, body any, output any) error {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("metorial: failed to encode request body: %w", err)
		}
		payload = encoded
	}

	headers := map[string]string{}
	if payload != nil {
		headers["Content-Type"] = "application/json"
	}

	response, err := fetch.Do(c.runtime, fetch.Request{
		Method:  method,
		Target:  target,
		Headers: headers,
		Body:    payload,
	})
	if err != nil {
		return describeError(target, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return describeError(target, responseError(response))
	}

	if output == nil || len(response.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Body, output); err != nil {
		return fmt.Errorf("metorial: failed to decode response from %s: %w", target, err)
	}

	return nil
}

func responseError(response *fetch.Response) *metorial.Error {
	apiErr := &metorial.Error{StatusCode: response.StatusCode}

	body := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{}
	if json.Unmarshal(response.Body, &body) == nil {
		apiErr.Code = body.Code
		apiErr.Message = strings.TrimSpace(body.Message)
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(response.StatusCode)
	}

	return apiErr
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

func listQuery(options ListOptions, extra url.Values) string {
	values := url.Values{}
	for key, items := range extra {
		for _, item := range items {
			values.Add(key, item)
		}
	}

	if options.Limit > 0 {
		values.Set("limit", strconv.FormatFloat(options.Limit, 'f', -1, 64))
	}
	if after := strings.TrimSpace(options.After); after != "" {
		values.Set("after", after)
	}
	if before := strings.TrimSpace(options.Before); before != "" {
		values.Set("before", before)
	}
	if cursor := strings.TrimSpace(options.Cursor); cursor != "" {
		values.Set("cursor", cursor)
	}

	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

func pathSegment(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}
