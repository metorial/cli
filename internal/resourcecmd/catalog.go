package resourcecmd

// PublicCatalog returns the resource groups exposed as top-level public CLI resources.
func PublicCatalog() []ResourceGroup {
	return []ResourceGroup{
		{
			Title: "Core Resources",
			Resources: []ResourceSpec{
				InstanceResource(),
				ProvidersResource(),
				DeploymentsResource(),
				ConfigsResource(),
				AuthConfigsResource(),
				IdentitiesResource(),
				ActorsResource(),
				IdentityCredentialsResource(),
			},
		},
	}
}
