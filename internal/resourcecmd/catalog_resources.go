package resourcecmd

func InstanceResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "instance",
		Singular: "instance",
		Short:    "Read the current instance",
		Operations: []OperationSpec{
			{
				Name:       OperationGet,
				Short:      "Get the current instance",
				SDKMapping: "sdk.Instance.Get()",
			},
		},
	}
}

func ProvidersResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "providers",
		Singular: "provider",
		Short:    "Browse providers",
		Operations: []OperationSpec{
			{
				Name:       OperationList,
				Short:      "List providers",
				SDKMapping: "sdk.Providers.List(params)",
				Flags: []FlagSpec{
					{Name: "limit", Type: FlagFloat, Target: "params.Limit", Usage: "Limit the number of providers"},
					{Name: "after", Type: FlagString, Target: "params.After", Usage: "Fetch providers after this cursor"},
					{Name: "before", Type: FlagString, Target: "params.Before", Usage: "Fetch providers before this cursor"},
					{Name: "cursor", Type: FlagString, Target: "params.Cursor", Usage: "Fetch providers using an opaque cursor"},
					{Name: "order", Type: FlagString, Target: "params.Order", Usage: "Sort order"},
					{Name: "id", Type: FlagStringSlice, Target: "params.Id", Usage: "Filter by one or more provider IDs", Repeated: true},
				},
			},
			{
				Name:       OperationGet,
				Short:      "Get a provider by ID",
				SDKMapping: "sdk.Providers.Get(providerId)",
				Args: []ArgumentSpec{
					{Name: "provider-id", Required: true, Description: "Provider ID"},
				},
			},
		},
	}
}

func ProviderListingsResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "provider-listings",
		Singular: "provider-listing",
		Short:    "Browse provider listings",
		Operations: []OperationSpec{
			{
				Name:       OperationList,
				Short:      "List provider listings",
				SDKMapping: "sdk.ProviderListings.List(params)",
				Flags: []FlagSpec{
					{Name: "limit", Type: FlagFloat, Target: "params.Limit", Usage: "Limit the number of listings"},
					{Name: "after", Type: FlagString, Target: "params.After", Usage: "Fetch listings after this cursor"},
					{Name: "before", Type: FlagString, Target: "params.Before", Usage: "Fetch listings before this cursor"},
					{Name: "cursor", Type: FlagString, Target: "params.Cursor", Usage: "Fetch listings using an opaque cursor"},
					{Name: "order", Type: FlagString, Target: "params.Order", Usage: "Sort order"},
					{Name: "id", Type: FlagStringSlice, Target: "params.Id", Usage: "Filter by listing ID", Repeated: true},
					{Name: "publisher-id", Type: FlagStringSlice, Target: "params.PublisherId", Usage: "Filter by publisher ID", Repeated: true},
					{Name: "provider-id", Type: FlagStringSlice, Target: "params.ProviderId", Usage: "Filter by provider ID", Repeated: true},
					{Name: "provider-category-id", Type: FlagStringSlice, Target: "params.ProviderCategoryId", Usage: "Filter by category ID", Repeated: true},
					{Name: "search", Type: FlagString, Target: "params.Search", Usage: "Search listings"},
				},
			},
			{
				Name:       OperationGet,
				Short:      "Get a provider listing by ID",
				SDKMapping: "sdk.ProviderListings.Get(providerListingId)",
				Args: []ArgumentSpec{
					{Name: "provider-listing-id", Required: true, Description: "Provider listing ID"},
				},
			},
		},
	}
}

func PublishersResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "publishers",
		Singular: "publisher",
		Short:    "Browse publishers",
		Operations: []OperationSpec{
			{
				Name:       OperationList,
				Short:      "List publishers",
				SDKMapping: "sdk.Publishers.List(params)",
				Flags: []FlagSpec{
					{Name: "limit", Type: FlagFloat, Target: "params.Limit", Usage: "Limit the number of publishers"},
					{Name: "after", Type: FlagString, Target: "params.After", Usage: "Fetch publishers after this cursor"},
					{Name: "before", Type: FlagString, Target: "params.Before", Usage: "Fetch publishers before this cursor"},
					{Name: "cursor", Type: FlagString, Target: "params.Cursor", Usage: "Fetch publishers using an opaque cursor"},
					{Name: "order", Type: FlagString, Target: "params.Order", Usage: "Sort order"},
					{Name: "id", Type: FlagStringSlice, Target: "params.Id", Usage: "Filter by publisher ID", Repeated: true},
				},
			},
			{
				Name:       OperationGet,
				Short:      "Get a publisher by ID",
				SDKMapping: "sdk.Publishers.Get(publisherId)",
				Args: []ArgumentSpec{
					{Name: "publisher-id", Required: true, Description: "Publisher ID"},
				},
			},
		},
	}
}
