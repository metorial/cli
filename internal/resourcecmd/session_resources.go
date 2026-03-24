package resourcecmd

func SessionsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "sessions",
		Singular: "session",
		Short:    "Manage sessions",
		Operations: []OperationSpec{
			{Name: OperationList, Short: "List sessions", SDKMapping: "sdk.Sessions.List(params)"},
			{Name: OperationGet, Short: "Get a session by ID", SDKMapping: "sdk.Sessions.Get(sessionId)", Args: []ArgumentSpec{{Name: "session-id", Required: true, Description: "Session ID"}}},
			{
				Name:       OperationCreate,
				Short:      "Create a session",
				SDKMapping: "sdk.Sessions.Create(body)",
				Flags: []FlagSpec{
					{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Session name"},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update a session",
				SDKMapping: "sdk.Sessions.Update(sessionId, body)",
				Args:       []ArgumentSpec{{Name: "session-id", Required: true, Description: "Session ID"}},
				Flags:      []FlagSpec{{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Updated session name"}},
			},
		},
	}
}

func SessionTemplatesResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "session-templates",
		Singular: "session-template",
		Short:    "Manage session templates",
		Operations: []OperationSpec{
			{Name: OperationList, Short: "List session templates", SDKMapping: "sdk.SessionTemplates.List(params)"},
			{Name: OperationGet, Short: "Get a session template by ID", SDKMapping: "sdk.SessionTemplates.Get(sessionTemplateId)", Args: []ArgumentSpec{{Name: "session-template-id", Required: true, Description: "Session template ID"}}},
			{
				Name:       OperationCreate,
				Short:      "Create a session template",
				SDKMapping: "sdk.SessionTemplates.Create(body)",
				Flags: []FlagSpec{
					{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Template name", Required: true},
				},
			},
			{
				Name:       OperationUpdate,
				Short:      "Update a session template",
				SDKMapping: "sdk.SessionTemplates.Update(sessionTemplateId, body)",
				Args:       []ArgumentSpec{{Name: "session-template-id", Required: true, Description: "Session template ID"}},
				Flags:      []FlagSpec{{Name: "name", Type: FlagString, Target: "body.Name", Usage: "Updated template name"}},
			},
		},
	}
}
