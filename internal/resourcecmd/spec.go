package resourcecmd

import (
	"fmt"
	"strings"
)

type OperationName string

const (
	OperationList          OperationName = "list"
	OperationGet           OperationName = "get"
	OperationCreate        OperationName = "create"
	OperationUpdate        OperationName = "update"
	OperationDelete        OperationName = "delete"
	OperationApprove       OperationName = "approve"
	OperationDeny          OperationName = "deny"
	OperationRevoke        OperationName = "revoke"
	OperationAddListing    OperationName = "add-listing"
	OperationRemoveListing OperationName = "remove-listing"
	OperationGetLogs       OperationName = "get-logs"
	OperationGetSchema     OperationName = "get-schema"
)

type FlagType string

const (
	FlagString      FlagType = "string"
	FlagBool        FlagType = "bool"
	FlagInt         FlagType = "int"
	FlagFloat       FlagType = "float"
	FlagStringSlice FlagType = "string-slice"
)

type ArgumentSpec struct {
	Name        string
	Required    bool
	Description string
}

type FlagSpec struct {
	Name      string
	Type      FlagType
	Target    string
	Usage     string
	Required  bool
	Repeated  bool
	Shorthand string
}

type OperationSpec struct {
	Name       OperationName
	Use        string
	Short      string
	Args       []ArgumentSpec
	Flags      []FlagSpec
	SDKMapping string
}

type ResourceSpec struct {
	Plural     string
	Singular   string
	Short      string
	Operations []OperationSpec
	Aliases    []string
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
	return unique(names)
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

func (r ResourceSpec) Operation(name OperationName) (OperationSpec, bool) {
	for _, operation := range r.Operations {
		if operation.Name == name {
			return operation, true
		}
	}

	return OperationSpec{}, false
}

func unique(values []string) []string {
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
