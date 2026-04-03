package resourcecmd

import (
	"fmt"
	"strings"
)

// ResourceGroup groups related resources for rollout or registration purposes.
type ResourceGroup struct {
	Title     string
	Resources []ResourceSpec
}

func (r ResourceSpec) Validate() error {
	if strings.TrimSpace(r.Plural) == "" {
		return fmt.Errorf("resource command: plural name is required")
	}

	if strings.TrimSpace(r.Singular) == "" {
		return fmt.Errorf("resource command: singular name is required for %q", r.Plural)
	}

	seen := map[string]struct{}{}
	for _, name := range r.Names() {
		if _, ok := seen[name]; ok {
			return fmt.Errorf("resource command: duplicate name %q for %q", name, r.Plural)
		}
		seen[name] = struct{}{}
	}

	for _, operation := range r.Operations {
		if strings.TrimSpace(string(operation.Name)) == "" {
			return fmt.Errorf("resource command: operation name is required for %q", r.Plural)
		}
		for _, flag := range operation.Flags {
			if strings.TrimSpace(flag.Name) == "" {
				return fmt.Errorf("resource command: operation %q on %q has a flag without a name", operation.Name, r.Plural)
			}
		}
	}

	return nil
}

func (r ResourceSpec) Names() []string {
	names := []string{r.Plural}
	if r.Singular != r.Plural {
		names = append(names, r.Singular)
	}
	names = append(names, r.Aliases...)
	return uniqueStrings(names)
}

func (r ResourceSpec) CobraAliases() []string {
	names := r.Names()
	if len(names) <= 1 {
		return nil
	}
	return names[1:]
}

func (r ResourceSpec) DefaultUse() string {
	return r.Plural
}

func (r ResourceSpec) APIPathPlural() string {
	if strings.TrimSpace(r.PathPlural) != "" {
		return strings.TrimSpace(r.PathPlural)
	}

	return r.Plural
}

func (r ResourceSpec) Operation(name OperationName) (OperationSpec, bool) {
	for _, operation := range r.Operations {
		if operation.Name == name {
			return operation, true
		}
	}

	return OperationSpec{}, false
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
