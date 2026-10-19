package consumerapi

import "time"

const (
	ProviderTypeProviderTemplate = "provider_template"
	ProviderTypeMagicMcpServer   = "magic_mcp_server"

	AvailabilityAvailableNow  = "available_now"
	AvailabilityRequestAccess = "request_access"

	SetupStatusPending    = "pending"
	SetupStatusSuccessful = "successful"
	SetupStatusExpired    = "expired"
	SetupStatusArchived   = "archived"
	SetupStatusDeleted    = "deleted"
)

type Pagination struct {
	HasMoreBefore bool `json:"has_more_before"`
	HasMoreAfter  bool `json:"has_more_after"`
}

type ListOptions struct {
	Limit  float64
	After  string
	Before string
	Cursor string
}

type Profile struct {
	Object     string                   `json:"object"`
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Email      string                   `json:"email"`
	ImageURL   string                   `json:"image_url"`
	ConsumerID string                   `json:"consumer_id"`
	UserID     *string                  `json:"user_id"`
	Status     string                   `json:"status"`
	Groups     []ProfileGroupAssignment `json:"groups"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
}

type ProfileGroupAssignment struct {
	Group       ConsumerGroup `json:"group"`
	AssignedVia string        `json:"assigned_via"`
}

type ConsumerGroup struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	IsDefault   bool    `json:"is_default"`
}

type ProviderGroup struct {
	Object      string    `json:"object"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Index       int       `json:"index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProviderGroupList struct {
	Items      []ProviderGroup `json:"items"`
	Pagination Pagination      `json:"pagination"`
}

type Provider struct {
	Object                  string   `json:"object"`
	ID                      string   `json:"id"`
	Type                    string   `json:"type"`
	Name                    string   `json:"name"`
	Description             *string  `json:"description"`
	Readme                  *string  `json:"readme"`
	Availability            string   `json:"availability"`
	HasPendingAccessRequest bool     `json:"has_pending_access_request"`
	ConsumerAccessIDs       []string `json:"consumer_access_ids"`

	ProviderTemplate *ProviderTemplatePreview `json:"provider_template,omitempty"`
	Provider         *ProviderDetail          `json:"provider,omitempty"`
	ConfigSchema     *JSONSchema              `json:"config_schema,omitempty"`
	AuthMethods      []AuthMethod             `json:"auth_methods,omitempty"`

	MagicMcpServer *MagicMcpServerPreview `json:"magic_mcp_server,omitempty"`
}

type ProviderList struct {
	Items      []Provider `json:"items"`
	Pagination Pagination `json:"pagination"`
}

type ProviderTemplatePreview struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type ProviderDetail struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	Description    *string                 `json:"description"`
	Slug           string                  `json:"slug"`
	Publisher      *ProviderPublisher      `json:"publisher"`
	CurrentVersion *ProviderCurrentVersion `json:"current_version"`
}

type ProviderPublisher struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProviderCurrentVersion struct {
	ID              string  `json:"id"`
	Version         string  `json:"version"`
	SpecificationID *string `json:"specification_id"`
}

type JSONSchema struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

type AuthMethod struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type ProviderSpecification struct {
	ID    string              `json:"id"`
	Key   string              `json:"key"`
	Name  string              `json:"name"`
	Tools []SpecificationTool `json:"tools"`
}

type SpecificationTool struct {
	ID          string  `json:"id"`
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type SetupSession struct {
	Object      string    `json:"object"`
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	URL         string    `json:"url"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type AccessRequest struct {
	Object            string    `json:"object"`
	ID                string    `json:"id"`
	Status            string    `json:"status"`
	Message           *string   `json:"message"`
	ResolutionMessage *string   `json:"resolution_message"`
	CreatedAt         time.Time `json:"created_at"`
}

type MagicMcpServerPreview struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type MagicMcpServerEndpoint struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
	URL   string `json:"url"`
}

type MagicMcpServer struct {
	Object             string                   `json:"object"`
	ID                 string                   `json:"id"`
	Status             string                   `json:"status"`
	Source             string                   `json:"source"`
	ProviderTemplateID *string                  `json:"provider_template_id"`
	Endpoints          []MagicMcpServerEndpoint `json:"endpoints"`
	Providers          []MagicMcpServerProvider `json:"providers"`
	Name               *string                  `json:"name"`
	Description        *string                  `json:"description"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

type MagicMcpServerList struct {
	Items      []MagicMcpServer `json:"items"`
	Pagination Pagination       `json:"pagination"`
}

type MagicMcpServerProvider struct {
	ID       string                         `json:"id"`
	Status   string                         `json:"status"`
	Name     string                         `json:"name"`
	Provider *MagicMcpServerProviderPreview `json:"provider"`
}

type MagicMcpServerProviderPreview struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Slug        string  `json:"slug"`
}

type UpdateMagicMcpServerInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type DeployInput struct {
	IntegrationSetupSessionID string  `json:"integration_setup_session_id"`
	Name                      *string `json:"name,omitempty"`
	Description               *string `json:"description,omitempty"`
}

type MagicMcpToken struct {
	Object      string                 `json:"object"`
	ID          string                 `json:"id"`
	Status      string                 `json:"status"`
	Secret      string                 `json:"secret"`
	Name        *string                `json:"name"`
	Description *string                `json:"description"`
	Server      *MagicMcpServerPreview `json:"server"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type MagicMcpTokenList struct {
	Items      []MagicMcpToken `json:"items"`
	Pagination Pagination      `json:"pagination"`
}

type CreateMagicMcpTokenInput struct {
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	MagicMcpServerID *string `json:"magic_mcp_server_id,omitempty"`
}
