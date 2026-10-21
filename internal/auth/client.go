package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	AccessToken     string               `json:"access_token"`
	TokenType       string               `json:"token_type"`
	RefreshToken    string               `json:"refresh_token"`
	ExpiresIn       int                  `json:"expires_in"`
	ClientID        string               `json:"client_id"`
	User            UserInfo             `json:"user"`
	Organization    OrgInfo              `json:"organization"`
	Scope           ScopeValues          `json:"scope"`
	SubjectType     string               `json:"subject_type"`
	ConsumerProfile *ConsumerProfileInfo `json:"consumer_profile"`
	Portal          *PortalInfo          `json:"portal"`
	Instance        *InstanceInfo        `json:"instance"`
}

type ConsumerProfileInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type PortalInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type InstanceInfo struct {
	ID string `json:"id"`
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
	Message      string `json:"message"`
	Code         string `json:"code"`
	Reason       string `json:"reason"`
	RetryAfter   int    `json:"retry_after"`
}

type SignupResponse struct {
	TokenResponse
	Organization SignupOrgInfo      `json:"organization"`
	Project      SignupResourceInfo `json:"project"`
	Instance     SignupResourceInfo `json:"instance"`
	Region       string             `json:"region"`
	Claim        ClaimInfo          `json:"claim"`
}

type SignupRegion struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TraceURL string `json:"trace_url"`
}

type SignupRegionsResponse struct {
	Regions           []SignupRegion `json:"regions"`
	RecommendedRegion string         `json:"recommended_region"`
}

type SignupOrgInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type SignupResourceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type ClaimInfo struct {
	Status string `json:"status"`
	Token  string `json:"token"`
	URL    string `json:"url"`
}

type OnboardingStatusResponse struct {
	Status        string        `json:"status"`
	ClaimURL      string        `json:"claim_url"`
	ClaimEmailSet bool          `json:"claim_email_set"`
	Organization  SignupOrgInfo `json:"organization"`
	User          *UserInfo     `json:"user"`
}

type ClaimEmailResponse struct {
	Status         string `json:"status"`
	ClaimEmail     string `json:"claim_email"`
	NextReminderAt string `json:"next_reminder_at"`
}

func NewClient(baseURL *url.URL) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) StartCLIAuth(subjectType string) (*StartResponse, error) {
	response := &StartResponse{}
	values := url.Values{}
	if subjectType = strings.TrimSpace(subjectType); subjectType != "" {
		values.Set("subject_type", subjectType)
	}
	if err := c.postForm("/cli/auth/start", values, response); err != nil {
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

func (c *Client) CLISignupRegions() (*SignupRegionsResponse, error) {
	response := &SignupRegionsResponse{}
	if err := c.getJSON("/cli/onboarding/regions", response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) CLISignup(organizationName, region string) (*SignupResponse, error) {
	response := &SignupResponse{}
	body := map[string]any{}
	if name := strings.TrimSpace(organizationName); name != "" {
		body["organization_name"] = name
	}
	if region = strings.TrimSpace(region); region != "" {
		body["region"] = region
	}
	if err := c.postJSON("/cli/onboarding/signup", "", body, response); err != nil {
		return nil, err
	}

	// The outer organization/instance fields shadow the embedded token fields during decoding.
	response.TokenResponse.Organization = OrgInfo{ID: response.Organization.ID, Name: response.Organization.Name}
	response.TokenResponse.Instance = &InstanceInfo{ID: response.Instance.ID}

	return response, nil
}

func (c *Client) CLIOnboardingStatus(accessToken, orgID, claimToken string) (*OnboardingStatusResponse, error) {
	response := &OnboardingStatusResponse{}
	body := map[string]any{
		"organization_id": strings.TrimSpace(orgID),
		"claim_token":     strings.TrimSpace(claimToken),
	}
	if err := c.postJSON("/cli/onboarding/status", accessToken, body, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) CLIClaimEmail(accessToken, orgID, claimToken, email string) (*ClaimEmailResponse, error) {
	response := &ClaimEmailResponse{}
	body := map[string]any{
		"organization_id": strings.TrimSpace(orgID),
		"claim_token":     strings.TrimSpace(claimToken),
		"email":           strings.TrimSpace(email),
	}
	if err := c.postJSON("/cli/onboarding/claim-email", accessToken, body, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) getJSON(path string, output any) error {
	requestURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequest(http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("metorial: failed to create request: %w", err)
	}

	request.Header.Set("Accept", "application/json")

	return c.do(request, output)
}

func (c *Client) postJSON(path string, accessToken string, body any, output any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode request: %w", err)
	}

	requestURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequest(http.MethodPost, requestURL.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("metorial: failed to create request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(accessToken); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	return c.do(request, output)
}

func (c *Client) postForm(path string, values url.Values, output any) error {
	requestURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequest(http.MethodPost, requestURL.String(), bytes.NewBufferString(values.Encode()))
	if err != nil {
		return fmt.Errorf("metorial: failed to create auth request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.do(request, output)
}

func (c *Client) do(request *http.Request, output any) error {
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
		if strings.TrimSpace(apiError.ErrorCode) == "" {
			apiError.ErrorCode = apiError.Code
		}
		if strings.TrimSpace(apiError.ErrorMessage) == "" {
			apiError.ErrorMessage = firstNonEmpty(apiError.Message, http.StatusText(response.StatusCode))
		}
		if apiError.RetryAfter == 0 {
			if retryAfter, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil {
				apiError.RetryAfter = retryAfter
			}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
