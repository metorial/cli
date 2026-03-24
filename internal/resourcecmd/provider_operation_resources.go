package resourcecmd

func ProviderDeploymentsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "provider-deployments",
		Singular: "provider-deployment",
		Short:    "Manage provider deployments",
		Operations: []OperationSpec{
			{
				Name:       OperationList,
				Short:      "List provider deployments",
				SDKMapping: "sdk.ProviderDeployments.List(params)",
				Flags: []FlagSpec{
					{Name: "limit", Type: FlagFloat, Target: "params.Limit", Usage: "Limit the number of deployments"},
					{Name: "after", Type: FlagString, Target: "params.After", Usage: "Fetch deployments after this cursor"},
					{Name: "before", Type: FlagString, Target: "params.Before", Usage: "Fetch deployments before this cursor"},
					{Name: "cursor", Type: FlagString, Target: "params.Cursor", Usage: "Fetch deployments using an opaque cursor"},
					{Name: "order", Type: FlagString, Target: "params.Order", Usage: "Sort order"},
					{Name: "id", Type: FlagStringSlice, Target: "params.Id", Usage: "Filter by deployment ID", Repeated: true},
					{Name: "provider-id", Type: FlagStringSlice, Target: "params.ProviderId", Usage: "Filter by provider ID", Repeated: true},
					{Name: "provider-version-id", Type: FlagStringSlice, Target: "params.ProviderVersionId", Usage: "Filter by provider version ID", Repeated: true},
					{Name: "status", Type: FlagStringSlice, Target: "params.Status", Usage: "Filter by deployment status", Repeated: true},
					{Name: "search", Type: FlagString, Target: "params.Search", Usage: "Search by name or description"},
				},
			},
			{
				Name:       OperationGet,
				Short:      "Get a provider deployment by ID",
				SDKMapping: "sdk.ProviderDeployments.Get(providerDeploymentId)",
				Args:       []ArgumentSpec{{Name: "provider-deployment-id", Required: true, Description: "Provider deployment ID"}},
			},
			{
				Name:       OperationCreate,
				Short:      "Create a provider deployment",
				SDKMapping: "sdk.ProviderDeployments.Create(body)",
				Flags: []FlagSpec{
					{Name: "provider-id", Type: FlagString, Target: "body.ProviderId", Usage: "Provider to deploy", Required: true},
					{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Deployment name"},
					{Name: "description", Type: FlagString, Target: "body.Description", Usage: "Deployment description"},
					{Name: "locked-provider-version-id", Type: FlagString, Target: "body.LockedProviderVersionId", Usage: "Pinned provider version"},
					{Name: "provider-config-id", Type: FlagString, Target: "body.ProviderConfigId", Usage: "Existing provider config ID"},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update a provider deployment",
				SDKMapping: "sdk.ProviderDeployments.Update(providerDeploymentId, body)",
				Args:       []ArgumentSpec{{Name: "provider-deployment-id", Required: true, Description: "Provider deployment ID"}},
				Flags: []FlagSpec{
					{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Deployment name"},
					{Name: "description", Type: FlagString, Target: "body.Description", Usage: "Deployment description"},
				},
			},
			{
				Name:       OperationDelete,
				Short:      "Delete a provider deployment",
				SDKMapping: "sdk.ProviderDeployments.Delete(providerDeploymentId)",
				Args:       []ArgumentSpec{{Name: "provider-deployment-id", Required: true, Description: "Provider deployment ID"}},
			},
		},
	}
}

func ProviderConfigsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "provider-configs",
		Singular: "provider-config",
		Short:    "Manage provider configs",
		Operations: []OperationSpec{
			{Name: OperationList, Short: "List provider configs", SDKMapping: "sdk.ProviderDeploymentsConfigs.List(params)"},
			{Name: OperationGet, Short: "Get a provider config by ID", SDKMapping: "sdk.ProviderDeploymentsConfigs.Get(providerConfigId)", Args: []ArgumentSpec{{Name: "provider-config-id", Required: true, Description: "Provider config ID"}}},
			{
				Name:       OperationCreate,
				Short:      "Create a provider config",
				SDKMapping: "sdk.ProviderDeploymentsConfigs.Create(body)",
				Flags: []FlagSpec{
					{Name: "provider-deployment-id", Type: FlagString, Target: "body.ProviderDeploymentId", Usage: "Provider deployment ID", Required: true},
					{Name: "key", Type: FlagString, Target: "body.Key", Usage: "Config key", Required: true},
					{Name: "value", Type: FlagString, Target: "body.Value", Usage: "Config value"},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update a provider config",
				SDKMapping: "sdk.ProviderDeploymentsConfigs.Update(providerConfigId, body)",
				Args:       []ArgumentSpec{{Name: "provider-config-id", Required: true, Description: "Provider config ID"}},
				Flags:      []FlagSpec{{Name: "value", Type: FlagString, Target: "body.Value", Usage: "Updated config value"}},
			},
			{Name: OperationDelete, Short: "Delete a provider config", SDKMapping: "sdk.ProviderDeploymentsConfigs.Delete(providerConfigId)", Args: []ArgumentSpec{{Name: "provider-config-id", Required: true, Description: "Provider config ID"}}},
		},
	}
}

func ProviderAuthConfigsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "provider-auth-configs",
		Singular: "provider-auth-config",
		Short:    "Manage provider auth configs",
		Operations: []OperationSpec{
			{Name: OperationList, Short: "List provider auth configs", SDKMapping: "sdk.ProviderDeploymentsAuthConfigs.List(params)"},
			{Name: OperationGet, Short: "Get a provider auth config by ID", SDKMapping: "sdk.ProviderDeploymentsAuthConfigs.Get(providerAuthConfigId)", Args: []ArgumentSpec{{Name: "provider-auth-config-id", Required: true, Description: "Provider auth config ID"}}},
			{
				Name:       OperationCreate,
				Short:      "Create a provider auth config",
				SDKMapping: "sdk.ProviderDeploymentsAuthConfigs.Create(body)",
				Flags: []FlagSpec{
					{Name: "provider-deployment-id", Type: FlagString, Target: "body.ProviderDeploymentId", Usage: "Provider deployment ID", Required: true},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update a provider auth config",
				SDKMapping: "sdk.ProviderDeploymentsAuthConfigs.Update(providerAuthConfigId, body)",
				Args:       []ArgumentSpec{{Name: "provider-auth-config-id", Required: true, Description: "Provider auth config ID"}},
			},
			{Name: OperationDelete, Short: "Delete a provider auth config", SDKMapping: "sdk.ProviderDeploymentsAuthConfigs.Delete(providerAuthConfigId)", Args: []ArgumentSpec{{Name: "provider-auth-config-id", Required: true, Description: "Provider auth config ID"}}},
		},
	}
}
