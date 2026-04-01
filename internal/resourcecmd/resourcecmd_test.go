package resourcecmd

import "testing"

func TestResourceSpecNames(t *testing.T) {
	t.Parallel()

	spec := ResourceSpec{
		Plural:   "providers",
		Singular: "provider",
		Aliases:  []string{"provider-catalog"},
	}

	names := spec.Names()
	if len(names) != 3 {
		t.Fatalf("Names() length = %d, want 3", len(names))
	}
	if names[0] != "providers" || names[1] != "provider" {
		t.Fatalf("Names() = %#v", names)
	}
}

func TestPublicResourcePlanContainsConfigs(t *testing.T) {
	t.Parallel()

	found := false
	for _, group := range PublicResourcePlan() {
		for _, resource := range group.Resources {
			if resource.Plural == "configs" && resource.Singular == "config" && resource.PathPlural == "provider-configs" {
				found = true
			}
		}
	}

	if !found {
		t.Fatalf("PublicResourcePlan() missing configs resource")
	}
}

func TestProvidersResourceHasExplicitFlagMapping(t *testing.T) {
	t.Parallel()

	resource := ProvidersResource()
	operation, ok := resource.Operation(OperationList)
	if !ok {
		t.Fatalf("ProvidersResource() missing list operation")
	}

	found := false
	for _, flag := range operation.Flags {
		if flag.Name == "id" && flag.Target == "params.Id" {
			found = true
		}
	}

	if !found {
		t.Fatalf("ProvidersResource() list operation missing explicit id flag mapping")
	}
}
