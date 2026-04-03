package resources

import (
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/spf13/cobra"
)

func AddSessionCommands(root *cobra.Command, ctx commandutil.Context) error {
	application := ctx.App
	rootOptions := newRootOptionsView(ctx.Options)

	sessionsCommand, err := resourcecmd.NewResourceCommand(sessionResourceSpec(), func(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) (*cobra.Command, error) {
		return newPublicResourceAction(application, rootOptions, resource, operation)
	})
	if err != nil {
		return err
	}
	commandutil.SetCommandCategory(sessionsCommand, commandutil.CommandCategoryResource)
	commandutil.ConfigureCommand(sessionsCommand)

	for _, resource := range []resourcecmd.ResourceSpec{
		sessionMessagesResourceSpec(),
		sessionParticipantsResourceSpec(),
		sessionConnectionsResourceSpec(),
		sessionEventsResourceSpec(),
		sessionProvidersResourceSpec(),
		sessionErrorsResourceSpec(),
	} {
		command, err := resourcecmd.NewResourceCommand(resource, func(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) (*cobra.Command, error) {
			return newPublicResourceAction(application, rootOptions, resource, operation)
		})
		if err != nil {
			return err
		}
		commandutil.ConfigureCommand(command)
		sessionsCommand.AddCommand(command)
	}

	root.AddCommand(sessionsCommand)

	sessionTemplatesCommand, err := resourcecmd.NewResourceCommand(sessionTemplatesResourceSpec(), func(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) (*cobra.Command, error) {
		return newPublicResourceAction(application, rootOptions, resource, operation)
	})
	if err != nil {
		return err
	}
	commandutil.SetCommandCategory(sessionTemplatesCommand, commandutil.CommandCategoryResource)
	commandutil.ConfigureCommand(sessionTemplatesCommand)

	templateProvidersCommand, err := resourcecmd.NewResourceCommand(sessionTemplateProvidersResourceSpec(), func(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) (*cobra.Command, error) {
		return newPublicResourceAction(application, rootOptions, resource, operation)
	})
	if err != nil {
		return err
	}
	commandutil.ConfigureCommand(templateProvidersCommand)
	sessionTemplatesCommand.AddCommand(templateProvidersCommand)
	root.AddCommand(sessionTemplatesCommand)

	return nil
}

func sessionResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:   "sessions",
		Singular: "session",
		Short:    "Manage sessions",
		Long:     "Create and inspect sessions and their related resources.",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List sessions",
				Long:  "List sessions for the selected instance.",
				Flags: append(resourcecmdPaginationFlags("sessions"),
					resourcecmdStringSliceFlag("status", "params.Status", "Filter by session status"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by session ID"),
					resourcecmdStringSliceFlag("session-template-id", "params.SessionTemplateId", "Filter by session template ID"),
					resourcecmdStringSliceFlag("session-provider-id", "params.SessionProviderId", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("provider-id", "params.ProviderId", "Filter by provider ID"),
					resourcecmdStringSliceFlag("provider-deployment-id", "params.ProviderDeploymentId", "Filter by provider deployment ID"),
					resourcecmdStringSliceFlag("provider-config-id", "params.ProviderConfigId", "Filter by provider config ID"),
					resourcecmdStringSliceFlag("provider-auth-config-id", "params.ProviderAuthConfigId", "Filter by provider auth config ID"),
				),
				Examples: []string{"metorial sessions list"},
			},
			{
				Name:     resourcecmd.OperationGet,
				Short:    "Get a session",
				Long:     "Get a single session by ID.",
				Args:     []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "", "Session ID. Find one with `metorial sessions list`.", true)},
				Examples: []string{"metorial sessions get ses_123"},
			},
			{
				Name:  resourcecmd.OperationCreate,
				Short: "Create a session",
				Long:  "Create a session. Use repeatable `--provider` flags or `--provider-file` to define the providers included in the session.",
				Flags: []resourcecmd.FlagSpec{
					resourcecmdStringFlag("name", "body.Name", "Optional display name", false),
					resourcecmdStringFlag("description", "body.Description", "Optional description", false),
					resourcecmdJSONFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					resourcecmdJSONFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
					resourcecmdProviderFlag(),
					resourcecmdProviderFileFlag(),
				},
				Examples: []string{
					"metorial sessions create --provider deployment=dep_123",
					"metorial sessions create --provider deployment=dep_123,config=cfg_123,auth-config=acf_123",
				},
			},
			{
				Name:  resourcecmd.OperationUpdate,
				Short: "Update a session",
				Long:  "Update mutable fields on a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdStringFlag("name", "body.Name", "Updated display name", false),
					resourcecmdStringFlag("description", "body.Description", "Updated description", false),
					resourcecmdJSONFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					resourcecmdJSONFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
				},
				Examples: []string{"metorial sessions update ses_123 --description \"Updated description\""},
			},
		},
	}
}

func sessionMessagesResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "messages",
		Singular:   "message",
		Short:      "Inspect session messages",
		PathPlural: "session-messages",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List messages for a session",
				Long:  "List messages for a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "params.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: append(resourcecmdPaginationFlags("messages"),
					resourcecmdStringSliceFlag("type", "params.Type", "Filter by message type"),
					resourcecmdStringSliceFlag("source", "params.Source", "Filter by message source"),
					resourcecmdStringSliceFlag("hierarchy", "params.Hierarchy", "Filter by message hierarchy"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by message ID"),
					resourcecmdStringSliceFlag("session-provider-id", "params.SessionProviderId", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("session-connection-id", "params.SessionConnectionId", "Filter by session connection ID"),
					resourcecmdStringSliceFlag("provider-run-id", "params.ProviderRunId", "Filter by provider run ID"),
					resourcecmdStringSliceFlag("error-id", "params.ErrorId", "Filter by error ID"),
					resourcecmdStringSliceFlag("participant-id", "params.ParticipantId", "Filter by participant ID"),
					resourcecmdStringSliceFlag("parent-message-id", "params.ParentMessageId", "Filter by parent message ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session message",
				Long:  "Get a single session message by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-message-id", "", "Session message ID.", true)},
			},
		},
	}
}

func sessionParticipantsResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "participants",
		Singular:   "participant",
		Short:      "Inspect session participants",
		PathPlural: "session-participants",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List participants for a session",
				Long:  "List participants for a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "params.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: append(resourcecmdPaginationFlags("participants"),
					resourcecmdStringSliceFlag("type", "params.Type", "Filter by participant type"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by participant ID"),
					resourcecmdStringSliceFlag("session-connection-id", "params.SessionConnectionId", "Filter by session connection ID"),
					resourcecmdStringSliceFlag("session-message-id", "params.SessionMessageId", "Filter by session message ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session participant",
				Long:  "Get a single session participant by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-participant-id", "", "Session participant ID.", true)},
			},
		},
	}
}

func sessionConnectionsResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "connections",
		Singular:   "connection",
		Short:      "Inspect session connections",
		PathPlural: "session-connections",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List connections for a session",
				Long:  "List connections for a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "params.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: append(resourcecmdPaginationFlags("connections"),
					resourcecmdStringSliceFlag("status", "params.Status", "Filter by connection status"),
					resourcecmdStringSliceFlag("connection-state", "params.ConnectionState", "Filter by connection state"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by session connection ID"),
					resourcecmdStringSliceFlag("session-provider-id", "params.SessionProviderId", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("participant-id", "params.ParticipantId", "Filter by participant ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session connection",
				Long:  "Get a single session connection by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-connection-id", "", "Session connection ID.", true)},
			},
		},
	}
}

func sessionEventsResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "events",
		Singular:   "event",
		Short:      "Inspect session events",
		PathPlural: "session-events",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List events for a session",
				Long:  "List events for a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "params.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: append(resourcecmdPaginationFlags("events"),
					resourcecmdStringSliceFlag("type", "params.Type", "Filter by event type"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by event ID"),
					resourcecmdStringSliceFlag("session-provider-id", "params.SessionProviderId", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("session-connection-id", "params.SessionConnectionId", "Filter by session connection ID"),
					resourcecmdStringSliceFlag("provider-run-id", "params.ProviderRunId", "Filter by provider run ID"),
					resourcecmdStringSliceFlag("session-message-id", "params.SessionMessageId", "Filter by session message ID"),
					resourcecmdStringSliceFlag("session-error-id", "params.SessionErrorId", "Filter by session error ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session event",
				Long:  "Get a single session event by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-event-id", "", "Session event ID.", true)},
			},
		},
	}
}

func sessionProvidersResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "providers",
		Singular:   "provider",
		Short:      "Manage session providers",
		PathPlural: "session-providers",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List providers for a session",
				Long:  "List providers connected to a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "params.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: append(resourcecmdPaginationFlags("session providers"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("session-template-id", "params.SessionTemplateId", "Filter by session template ID"),
					resourcecmdStringSliceFlag("provider-id", "params.ProviderId", "Filter by provider ID"),
					resourcecmdStringSliceFlag("provider-deployment-id", "params.ProviderDeploymentId", "Filter by provider deployment ID"),
					resourcecmdStringSliceFlag("provider-config-id", "params.ProviderConfigId", "Filter by provider config ID"),
					resourcecmdStringSliceFlag("provider-auth-config-id", "params.ProviderAuthConfigId", "Filter by provider auth config ID"),
					resourcecmdStringSliceFlag("status", "params.Status", "Filter by provider status"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session provider",
				Long:  "Get a single session provider by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-provider-id", "", "Session provider ID.", true)},
			},
			{
				Name:  resourcecmd.OperationCreate,
				Short: "Create a session provider",
				Long:  "Add a provider to a session using the currently supported API shape.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "body.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdJSONFlag("tool-filters", "body.ToolFilters", "Inline JSON value for tool filters"),
					resourcecmdJSONFileFlag("tool-filters-file", "body.ToolFilters", "Read tool filters JSON from a file"),
				},
			},
			{
				Name:  resourcecmd.OperationUpdate,
				Short: "Update a session provider",
				Long:  "Update tool filters on a session provider.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-provider-id", "", "Session provider ID.", true)},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdJSONFlag("tool-filters", "body.ToolFilters", "Inline JSON value for tool filters"),
					resourcecmdJSONFileFlag("tool-filters-file", "body.ToolFilters", "Read tool filters JSON from a file"),
				},
			},
			{
				Name:  resourcecmd.OperationDelete,
				Short: "Delete a session provider",
				Long:  "Remove a provider from a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-provider-id", "", "Session provider ID.", true)},
			},
		},
	}
}

func sessionErrorsResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "errors",
		Singular:   "error",
		Short:      "Inspect session errors",
		PathPlural: "session-errors",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List errors for a session",
				Long:  "List errors for a session.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-id", "params.SessionId", "Session ID. Find one with `metorial sessions list`.", true)},
				Flags: append(resourcecmdPaginationFlags("errors"),
					resourcecmdStringSliceFlag("type", "params.Type", "Filter by error type"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by session error ID"),
					resourcecmdStringSliceFlag("session-provider-id", "params.SessionProviderId", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("session-connection-id", "params.SessionConnectionId", "Filter by session connection ID"),
					resourcecmdStringSliceFlag("provider-run-id", "params.ProviderRunId", "Filter by provider run ID"),
					resourcecmdStringSliceFlag("provider-id", "params.ProviderId", "Filter by provider ID"),
					resourcecmdStringSliceFlag("session-message-id", "params.SessionMessageId", "Filter by session message ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session error",
				Long:  "Get a single session error by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-error-id", "", "Session error ID.", true)},
			},
		},
	}
}

func sessionTemplatesResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:   "session-templates",
		Singular: "session-template",
		Short:    "Manage session templates",
		Long:     "Create and inspect reusable session templates.",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List session templates",
				Long:  "List session templates for the selected instance.",
				Flags: append(resourcecmdPaginationFlags("session templates"),
					resourcecmdStringSliceFlag("status", "params.Status", "Filter by template status"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by session template ID"),
					resourcecmdStringSliceFlag("session-id", "params.SessionId", "Filter by session ID"),
					resourcecmdStringSliceFlag("session-provider-id", "params.SessionProviderId", "Filter by session provider ID"),
					resourcecmdStringSliceFlag("provider-id", "params.ProviderId", "Filter by provider ID"),
					resourcecmdStringSliceFlag("provider-deployment-id", "params.ProviderDeploymentId", "Filter by provider deployment ID"),
					resourcecmdStringSliceFlag("provider-config-id", "params.ProviderConfigId", "Filter by provider config ID"),
					resourcecmdStringSliceFlag("provider-auth-config-id", "params.ProviderAuthConfigId", "Filter by provider auth config ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session template",
				Long:  "Get a single session template by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-template-id", "", "Session template ID. Find one with `metorial session-templates list`.", true)},
			},
			{
				Name:  resourcecmd.OperationCreate,
				Short: "Create a session template",
				Long:  "Create a session template. Use repeatable `--provider` flags or `--provider-file` to define template providers.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("name", "body.Name", "Template name.", true)},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdStringFlag("description", "body.Description", "Optional description", false),
					resourcecmdJSONFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					resourcecmdJSONFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
					resourcecmdProviderFlag(),
					resourcecmdProviderFileFlag(),
				},
				Examples: []string{
					`metorial session-templates create "Default GitHub session" --provider deployment=dep_123`,
				},
			},
			{
				Name:  resourcecmd.OperationUpdate,
				Short: "Update a session template",
				Long:  "Update mutable fields on a session template.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-template-id", "", "Session template ID. Find one with `metorial session-templates list`.", true)},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdStringFlag("name", "body.Name", "Updated template name", false),
					resourcecmdStringFlag("description", "body.Description", "Updated description", false),
					resourcecmdJSONFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					resourcecmdJSONFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
				},
			},
		},
	}
}

func sessionTemplateProvidersResourceSpec() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "providers",
		Singular:   "provider",
		Short:      "Manage session template providers",
		PathPlural: "session-template-providers",
		Operations: []resourcecmd.OperationSpec{
			{
				Name:  resourcecmd.OperationList,
				Short: "List providers for a session template",
				Long:  "List provider configurations attached to a session template.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-template-id", "params.SessionTemplateId", "Session template ID. Find one with `metorial session-templates list`.", true)},
				Flags: append(resourcecmdPaginationFlags("session template providers"),
					resourcecmdStringSliceFlag("status", "params.Status", "Filter by provider status"),
					resourcecmdStringSliceFlag("id", "params.Id", "Filter by session template provider ID"),
					resourcecmdStringSliceFlag("provider-id", "params.ProviderId", "Filter by provider ID"),
					resourcecmdStringSliceFlag("provider-deployment-id", "params.ProviderDeploymentId", "Filter by provider deployment ID"),
					resourcecmdStringSliceFlag("provider-config-id", "params.ProviderConfigId", "Filter by provider config ID"),
					resourcecmdStringSliceFlag("provider-auth-config-id", "params.ProviderAuthConfigId", "Filter by provider auth config ID"),
				),
			},
			{
				Name:  resourcecmd.OperationGet,
				Short: "Get a session template provider",
				Long:  "Get a single session template provider by ID.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-template-provider-id", "", "Session template provider ID.", true)},
			},
			{
				Name:  resourcecmd.OperationCreate,
				Short: "Create a session template provider",
				Long:  "Add a provider configuration to a session template.",
				Args: []resourcecmd.ArgumentSpec{
					resourcecmdArg("session-template-id", "body.SessionTemplateId", "Session template ID. Find one with `metorial session-templates list`.", true),
					resourcecmdArg("provider-deployment-id", "body.ProviderDeploymentId", "Provider deployment ID. Find one with `metorial deployments list`.", true),
				},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdStringFlag("provider-config-id", "body.ProviderConfigId", "Optional provider config ID", false),
					resourcecmdStringFlag("provider-config-vault-id", "body.ProviderConfigVaultId", "Optional provider config vault ID", false),
					resourcecmdStringFlag("provider-auth-config-id", "body.ProviderAuthConfigId", "Optional provider auth config ID", false),
					resourcecmdJSONFlag("tool-filters", "body.ToolFilters", "Inline JSON value for tool filters"),
					resourcecmdJSONFileFlag("tool-filters-file", "body.ToolFilters", "Read tool filters JSON from a file"),
				},
			},
			{
				Name:  resourcecmd.OperationUpdate,
				Short: "Update a session template provider",
				Long:  "Update tool filters on a session template provider.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-template-provider-id", "", "Session template provider ID.", true)},
				Flags: []resourcecmd.FlagSpec{
					resourcecmdJSONFlag("tool-filters", "body.ToolFilters", "Inline JSON value for tool filters"),
					resourcecmdJSONFileFlag("tool-filters-file", "body.ToolFilters", "Read tool filters JSON from a file"),
				},
			},
			{
				Name:  resourcecmd.OperationDelete,
				Short: "Delete a session template provider",
				Long:  "Remove a provider configuration from a session template.",
				Args:  []resourcecmd.ArgumentSpec{resourcecmdArg("session-template-provider-id", "", "Session template provider ID.", true)},
			},
		},
	}
}

func resourcecmdArg(name, target, description string, required bool) resourcecmd.ArgumentSpec {
	return resourcecmd.ArgumentSpec{Name: name, Target: target, Description: description, Required: required}
}

func resourcecmdStringFlag(name, target, usage string, required bool) resourcecmd.FlagSpec {
	return resourcecmd.FlagSpec{Name: name, Type: resourcecmd.FlagString, Target: target, Usage: usage, Required: required}
}

func resourcecmdStringSliceFlag(name, target, usage string) resourcecmd.FlagSpec {
	return resourcecmd.FlagSpec{Name: name, Type: resourcecmd.FlagStringSlice, Target: target, Usage: usage, Repeated: true}
}

func resourcecmdJSONFlag(name, target, usage string) resourcecmd.FlagSpec {
	return resourcecmd.FlagSpec{Name: name, Type: resourcecmd.FlagJSON, Target: target, Usage: usage}
}

func resourcecmdJSONFileFlag(name, target, usage string) resourcecmd.FlagSpec {
	return resourcecmd.FlagSpec{Name: name, Type: resourcecmd.FlagJSONFile, Target: target, Usage: usage}
}

func resourcecmdPaginationFlags(label string) []resourcecmd.FlagSpec {
	return []resourcecmd.FlagSpec{
		{Name: "limit", Type: resourcecmd.FlagFloat, Target: "params.Limit", Usage: "Limit the number of " + label},
		{Name: "after", Type: resourcecmd.FlagString, Target: "params.After", Usage: "Fetch " + label + " after this cursor"},
		{Name: "before", Type: resourcecmd.FlagString, Target: "params.Before", Usage: "Fetch " + label + " before this cursor"},
		{Name: "cursor", Type: resourcecmd.FlagString, Target: "params.Cursor", Usage: "Fetch " + label + " using an opaque cursor"},
		{Name: "order", Type: resourcecmd.FlagString, Target: "params.Order", Usage: "Sort order"},
	}
}

func resourcecmdProviderFlag() resourcecmd.FlagSpec {
	return resourcecmd.FlagSpec{Name: "provider", Type: resourcecmd.FlagStringSlice, Usage: "Provider spec in key=value form. Supported keys: deployment, config, config-vault, auth-config", Repeated: true}
}

func resourcecmdProviderFileFlag() resourcecmd.FlagSpec {
	return resourcecmd.FlagSpec{Name: "provider-file", Type: resourcecmd.FlagString, Usage: "Read a JSON array of provider specs from a file"}
}
