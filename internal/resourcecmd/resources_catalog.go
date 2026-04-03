package resourcecmd

func InstanceResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "instance",
		Singular: "instance",
		Short:    "Read the current instance",
		Operations: []OperationSpec{
			{
				Name:  OperationGet,
				Short: "Get the current instance",
				Long:  "Get the instance selected for authenticated API requests.",
			},
		},
	}
}

func ProvidersResource() ResourceSpec {
	return ResourceSpec{
		Plural:   "providers",
		Singular: "provider",
		Short:    "Browse providers",
		Long:     "Browse providers using provider listing search and render the underlying provider objects.",
		Operations: []OperationSpec{
			{
				Name:  OperationList,
				Short: "List providers",
				Long:  "List providers using the searchable provider listings endpoint and render the nested provider objects.",
				Args:  []ArgumentSpec{arg("search", "params.Search", "Search providers by name or description.", false)},
				Flags: append(
					paginationFlags("providers"),
					stringSliceFlag("id", "params.Id", "Filter by provider listing ID"),
					stringSliceFlag("provider-category-id", "params.ProviderCategoryId", "Filter by provider category ID"),
					stringSliceFlag("provider-collection-id", "params.ProviderCollectionId", "Filter by provider collection ID"),
					stringSliceFlag("provider-group-id", "params.ProviderGroupId", "Filter by provider group ID"),
					stringSliceFlag("publisher-id", "params.PublisherId", "Filter by publisher ID"),
					boolFlag("is-owner", "params.IsOwner", "Filter by owner visibility"),
					boolFlag("is-public", "params.IsPublic", "Filter by public listings"),
					boolFlag("is-verified", "params.IsVerified", "Filter by verified listings"),
					boolFlag("is-official", "params.IsOfficial", "Filter by official listings"),
					boolFlag("is-metorial", "params.IsMetorial", "Filter by Metorial-maintained listings"),
				),
				Examples: []string{
					"metorial providers list",
					"metorial providers list github",
				},
			},
			{
				Name:     OperationGet,
				Short:    "Get a provider",
				Long:     "Get a single provider by provider ID using provider listing lookup behind the scenes.",
				Args:     []ArgumentSpec{arg("provider-id", "", "Provider ID. Find one with `metorial providers list`.", true)},
				Examples: []string{"metorial providers get prv_123"},
			},
		},
	}
}
