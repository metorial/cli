package resourcecmd

type ResourceGroup struct {
	Title     string
	Resources []ResourceSpec
}

func PublicResourcePlan() []ResourceGroup {
	return []ResourceGroup{
		{
			Title: "Initial Rollout",
			Resources: []ResourceSpec{
				InstanceResource(),
				ProvidersResource(),
				ProviderListingsResource(),
				PublishersResource(),
				ProviderDeploymentsResource(),
				ProviderConfigsResource(),
				ProviderAuthConfigsResource(),
				SessionsResource(),
				SessionTemplatesResource(),
				IdentitiesResource(),
				IdentityCredentialsResource(),
			},
		},
	}
}
