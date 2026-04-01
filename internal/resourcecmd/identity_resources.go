package resourcecmd

func IdentitiesResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "identities",
		Singular: "identity",
		Short:    "Manage identities",
		Long:     "Create, inspect, update, and delete identities owned by actors.",
		Operations: []OperationSpec{
			{
				Name:  OperationList,
				Short: "List identities",
				Long:  "List identities for the selected instance. When provided, the optional search argument filters by name or description.",
				Args:  []ArgumentSpec{arg("search", "params.Search", "Search identities by name or description.", false)},
				Flags: append(paginationFlags("identities"), stringSliceFlag("status", "params.Status", "Filter by identity status"), stringSliceFlag("id", "params.Id", "Filter by identity ID"), stringSliceFlag("agent-id", "params.AgentId", "Filter by owner agent ID"), stringSliceFlag("actor-id", "params.ActorId", "Filter by owner actor ID")),
				Examples: []string{
					"metorial identities list",
					"metorial identities list github",
				},
			},
			{
				Name:  OperationGet,
				Short: "Get an identity",
				Long:  "Get a single identity by ID.",
				Args:  []ArgumentSpec{arg("identity-id", "", "Identity ID. Find one with `metorial identities list`.", true)},
				Examples: []string{
					"metorial identities get idn_123",
				},
			},
			{
				Name:  OperationCreate,
				Short: "Create an identity",
				Long:  "Create an identity owned by an existing actor.",
				Args: []ArgumentSpec{
					arg("actor-id", "body.ActorId", "Identity actor that will own the identity. Find one with `metorial actors list`.", true),
					arg("name", "body.Name", "Display name for the identity.", true),
				},
				Flags: []FlagSpec{
					stringFlag("description", "body.Description", "Optional description", false),
					jsonFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					jsonFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
				},
				Examples: []string{
					`metorial identities create act_123 "GitHub production"`,
					`metorial identities create act_123 "GitHub production" --description "Used for deploys"`,
				},
				SeeAlso: []string{"metorial actors list"},
			},
			{
				Name:  OperationUpdate,
				Short: "Update an identity",
				Long:  "Update mutable fields on an identity.",
				Args:  []ArgumentSpec{arg("identity-id", "", "Identity ID. Find one with `metorial identities list`.", true)},
				Flags: []FlagSpec{
					stringFlag("name", "body.Name", "Updated display name", false),
					stringFlag("description", "body.Description", "Updated description", false),
					jsonFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					jsonFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
				},
				Examples: []string{
					`metorial identities update idn_123 --name "GitHub staging"`,
				},
			},
			{
				Name:     OperationDelete,
				Short:    "Delete an identity",
				Long:     "Archive an identity by ID.",
				Args:     []ArgumentSpec{arg("identity-id", "", "Identity ID. Find one with `metorial identities list`.", true)},
				Examples: []string{"metorial identities delete idn_123"},
			},
		},
	}
}

func ActorsResource() ResourceSpec {
	return ResourceSpec{
		Plural:     "actors",
		Singular:   "actor",
		Short:      "Manage identity actors",
		Long:       "Create and manage identity actors that own identities.",
		PathPlural: "identity-actors",
		Operations: []OperationSpec{
			{
				Name:  OperationList,
				Short: "List actors",
				Long:  "List identity actors for the selected instance. When provided, the optional search argument filters by name or description.",
				Args:  []ArgumentSpec{arg("search", "params.Search", "Search actors by name or description.", false)},
				Flags: append(paginationFlags("actors"), stringSliceFlag("status", "params.Status", "Filter by actor status"), stringSliceFlag("id", "params.Id", "Filter by actor ID"), stringSliceFlag("agent-id", "params.AgentId", "Filter by linked agent ID"), stringSliceFlag("consumer-id", "params.ConsumerId", "Filter by linked consumer ID")),
				Examples: []string{
					"metorial actors list",
					"metorial actors list support",
				},
			},
			{
				Name:     OperationGet,
				Short:    "Get an actor",
				Long:     "Get a single actor by ID.",
				Args:     []ArgumentSpec{arg("actor-id", "", "Actor ID. Find one with `metorial actors list`.", true)},
				Examples: []string{"metorial actors get act_123"},
			},
			{
				Name:  OperationCreate,
				Short: "Create an actor",
				Long:  "Create a new identity actor.",
				Args: []ArgumentSpec{
					arg("type", "body.Type", "Actor type. Accepted values are `person` or `agent`.", true),
					arg("name", "body.Name", "Display name for the actor.", true),
				},
				Flags: []FlagSpec{
					stringFlag("description", "body.Description", "Optional description", false),
					jsonFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					jsonFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
				},
				Examples: []string{
					`metorial actors create person "Primary operator"`,
					`metorial actors create agent "Support bot" --description "Owns support identities"`,
				},
			},
			{
				Name:  OperationUpdate,
				Short: "Update an actor",
				Long:  "Update mutable fields on an actor.",
				Args:  []ArgumentSpec{arg("actor-id", "", "Actor ID. Find one with `metorial actors list`.", true)},
				Flags: []FlagSpec{
					stringFlag("name", "body.Name", "Updated display name", false),
					stringFlag("description", "body.Description", "Updated description", false),
					jsonFlag("metadata", "body.Metadata", "Inline JSON object for metadata"),
					jsonFileFlag("metadata-file", "body.Metadata", "Read metadata JSON from a file"),
				},
				Examples: []string{"metorial actors update act_123 --description \"Updated description\""},
			},
			{
				Name:     OperationDelete,
				Short:    "Delete an actor",
				Long:     "Archive an actor by ID.",
				Args:     []ArgumentSpec{arg("actor-id", "", "Actor ID. Find one with `metorial actors list`.", true)},
				Examples: []string{"metorial actors delete act_123"},
			},
		},
	}
}

func IdentityCredentialsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "identity-credentials",
		Singular: "identity-credential",
		Short:    "Manage identity credentials",
		Long:     "Create and inspect credentials that connect identities to provider resources.",
		Operations: []OperationSpec{
			{
				Name:  OperationList,
				Short: "List identity credentials",
				Long:  "List identity credentials for the selected instance.",
				Flags: append(paginationFlags("identity credentials"),
					stringSliceFlag("status", "params.Status", "Filter by credential status"),
					stringSliceFlag("id", "params.Id", "Filter by identity credential ID"),
					stringSliceFlag("agent-id", "params.AgentId", "Filter by owner agent ID"),
					stringSliceFlag("actor-id", "params.ActorId", "Filter by owner actor ID"),
					stringSliceFlag("identity-id", "params.IdentityId", "Filter by identity ID"),
					stringSliceFlag("provider-id", "params.ProviderId", "Filter by provider ID"),
					stringSliceFlag("provider-deployment-id", "params.ProviderDeploymentId", "Filter by provider deployment ID"),
					stringSliceFlag("provider-config-id", "params.ProviderConfigId", "Filter by provider config ID"),
					stringSliceFlag("provider-auth-config-id", "params.ProviderAuthConfigId", "Filter by provider auth config ID"),
				),
				Examples: []string{"metorial identity-credentials list --identity-id idn_123"},
			},
			{
				Name:     OperationGet,
				Short:    "Get an identity credential",
				Long:     "Get a single identity credential by ID.",
				Args:     []ArgumentSpec{arg("identity-credential-id", "", "Identity credential ID. Find one with `metorial identity-credentials list`.", true)},
				Examples: []string{"metorial identity-credentials get icr_123"},
			},
			{
				Name:  OperationCreate,
				Short: "Create an identity credential",
				Long:  "Create an identity credential attached to an existing identity.",
				Args:  []ArgumentSpec{arg("identity-id", "body.IdentityId", "Identity that will own the credential. Find one with `metorial identities list`.", true)},
				Flags: []FlagSpec{
					stringFlag("deployment-id", "body.DeploymentId", "Provider deployment to attach", false),
					stringFlag("config-id", "body.ConfigId", "Provider config to attach", false),
					stringFlag("auth-config-id", "body.AuthConfigId", "Provider auth config to attach", false),
					stringFlag("delegation-config-id", "body.DelegationConfigId", "Delegation config to apply", false),
				},
				Examples: []string{
					"metorial identity-credentials create idn_123 --deployment-id dep_123",
				},
				SeeAlso: []string{"metorial identities list", "metorial deployments list", "metorial configs list", "metorial auth-configs list"},
			},
			{
				Name:     OperationDelete,
				Short:    "Delete an identity credential",
				Long:     "Archive an identity credential by ID.",
				Args:     []ArgumentSpec{arg("identity-credential-id", "", "Identity credential ID. Find one with `metorial identity-credentials list`.", true)},
				Examples: []string{"metorial identity-credentials delete icr_123"},
			},
		},
	}
}
