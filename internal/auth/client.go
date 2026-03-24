package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type StartResponse struct {
	ID               string `json:"id"`
	Token            string `json:"token"`
	ExpiresIn        int    `json:"expires_in"`
	Interval         int    `json:"interval"`
	UserCode         string `json:"user_code"`
	AuthorizationURL string `json:"authorization_url"`
}

type TokenResponse struct {
	AccessToken  string      `json:"access_token"`
	TokenType    string      `json:"token_type"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	ClientID     string      `json:"client_id"`
	User         UserInfo    `json:"user"`
	Organization OrgInfo     `json:"organization"`
	Scope        ScopeValues `json:"scope"`
}

type UserInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type OrgInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ScopeValues []string

type Error struct {
	StatusCode   int
	ErrorCode    string `json:"error"`
	ErrorMessage string `json:"error_message"`
}

func NewClient(baseURL *url.URL) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) StartCLIAuth() (*StartResponse, error) {
	response := &StartResponse{}
	if err := c.postForm("/cli/auth/start", url.Values{}, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) CompleteCLIAuth(token string) (*TokenResponse, error) {
	response := &TokenResponse{}
	values := url.Values{}
	values.Set("token", strings.TrimSpace(token))
	if err := c.postForm("/cli/auth/complete", values, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) RefreshToken(clientID, refreshToken string) (*TokenResponse, error) {
	response := &TokenResponse{}
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("client_id", strings.TrimSpace(clientID))
	values.Set("refresh_token", strings.TrimSpace(refreshToken))
	if err := c.postForm("/oauth/token", values, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) postForm(path string, values url.Values, output any) error {
	requestURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequest(http.MethodPost, requestURL.String(), bytes.NewBufferString(values.Encode()))
	if err != nil {
		return fmt.Errorf("metorial: failed to create auth request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("metorial: authentication request failed: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("metorial: failed to read authentication response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		apiError := &Error{StatusCode: response.StatusCode}
		_ = json.Unmarshal(body, apiError)
		if strings.TrimSpace(apiError.ErrorMessage) == "" {
			apiError.ErrorMessage = http.StatusText(response.StatusCode)
		}
		return apiError
	}

	if output == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}

	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("metorial: failed to parse authentication response: %w", err)
	}

	return nil
}

func (s *ScopeValues) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = nil
		return nil
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		parts := strings.Fields(text)
		*s = append((*s)[:0], parts...)
		return nil
	}

	var values []string
	if err := json.Unmarshal(trimmed, &values); err != nil {
		return err
	}

	*s = values
	return nil
}

func (e *Error) Error() string {
	if strings.TrimSpace(e.ErrorMessage) != "" {
		return e.ErrorMessage
	}
	if strings.TrimSpace(e.ErrorCode) != "" {
		return e.ErrorCode
	}
	return "authentication failed"
}
