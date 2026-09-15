package admin

import "testing"

func TestCatalogIncludesCoreResources(t *testing.T) {
	t.Parallel()

	catalog := Catalog()
	if len(catalog) == 0 {
		t.Fatal("generated catalog is empty; run `make generate`")
	}

	foundProviders := false
	hasCredentials := false
	hasSetupShortcut := false

	for _, resource := range catalog {
		if resource.Plural == "providers" {
			foundProviders = true
		}

		if resource.Plural == "outposts" {
			for _, child := range resource.Children {
				if child.Plural != "credentials" {
					continue
				}
				for _, operation := range child.Operations {
					if operation.Name == "create" && operation.SDKMapping == "outposts.credentials.create" {
						hasCredentials = true
					}
				}
			}
		}

		if resource.Plural == "integrations" {
			for _, child := range resource.Children {
				for _, shortcut := range child.Shortcuts {
					if len(shortcut.Path) == 2 && shortcut.Path[0] == "integrations" && shortcut.Path[1] == "setup" && string(shortcut.Method) == "create" {
						hasSetupShortcut = true
					}
				}
			}
		}
	}

	if !foundProviders {
		t.Fatal("catalog missing providers")
	}
	if !hasCredentials {
		t.Fatal("catalog missing outposts credentials create")
	}
	if !hasSetupShortcut {
		t.Fatal("catalog missing integrations setup shortcut")
	}
}
