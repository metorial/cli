package resourcecmd

func IdentitiesResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "identities",
		Singular: "identity",
		Short:    "Manage identities",
		Operations: []OperationSpec{
			{Name: OperationList, Short: "List identities", SDKMapping: "sdk.Identities.List(params)"},
			{Name: OperationGet, Short: "Get an identity by ID", SDKMapping: "sdk.Identities.Get(identityId)", Args: []ArgumentSpec{{Name: "identity-id", Required: true, Description: "Identity ID"}}},
			{
				Name:       OperationCreate,
				Short:      "Create an identity",
				SDKMapping: "sdk.Identities.Create(body)",
				Flags: []FlagSpec{
					{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Identity name"},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update an identity",
				SDKMapping: "sdk.Identities.Update(identityId, body)",
				Args:       []ArgumentSpec{{Name: "identity-id", Required: true, Description: "Identity ID"}},
				Flags:      []FlagSpec{{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Updated identity name"}},
			},
			{Name: OperationDelete, Short: "Delete an identity", SDKMapping: "sdk.Identities.Delete(identityId)", Args: []ArgumentSpec{{Name: "identity-id", Required: true, Description: "Identity ID"}}},
		},
	}
}

func IdentityCredentialsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "identity-credentials",
		Singular: "identity-credential",
		Short:    "Manage identity credentials",
		Operations: []OperationSpec{
			{Name: OperationList, Short: "List identity credentials", SDKMapping: "sdk.IdentitiesCredentials.List(params)"},
			{Name: OperationGet, Short: "Get an identity credential by ID", SDKMapping: "sdk.IdentitiesCredentials.Get(identityCredentialId)", Args: []ArgumentSpec{{Name: "identity-credential-id", Required: true, Description: "Identity credential ID"}}},
			{
				Name:       OperationCreate,
				Short:      "Create an identity credential",
				SDKMapping: "sdk.IdentitiesCredentials.Create(body)",
				Flags: []FlagSpec{
					{Name: "identity-id", Type: FlagString, Target: "body.IdentityId", Usage: "Identity ID", Required: true},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update an identity credential",
				SDKMapping: "sdk.IdentitiesCredentials.Update(identityCredentialId, body)",
				Args:       []ArgumentSpec{{Name: "identity-credential-id", Required: true, Description: "Identity credential ID"}},
			},
			{Name: OperationDelete, Short: "Delete an identity credential", SDKMapping: "sdk.IdentitiesCredentials.Delete(identityCredentialId)", Args: []ArgumentSpec{{Name: "identity-credential-id", Required: true, Description: "Identity credential ID"}}},
		},
	}
}
