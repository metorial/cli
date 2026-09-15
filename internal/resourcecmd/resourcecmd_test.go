package resourcecmd

import (
	"testing"

	"github.com/spf13/cobra"
)

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

func TestNewResourceCommandNestsChildren(t *testing.T) {
	t.Parallel()

	spec := ResourceSpec{
		Plural:   "sessions",
		Singular: "sessions",
		Short:    "Manage sessions",
		Operations: []OperationSpec{
			{Name: OperationList, Method: "GET", Path: "/sessions", Short: "List sessions"},
		},
		Children: []ResourceSpec{
			{
				Plural:   "messages",
				Singular: "messages",
				Short:    "Session messages",
				Operations: []OperationSpec{
					{Name: OperationList, Method: "GET", Path: "/session-messages", Short: "List messages"},
				},
			},
		},
	}

	command, err := NewResourceCommand(spec, func(resource ResourceSpec, operation OperationSpec) (*cobra.Command, error) {
		return NewPlaceholderAction(resource, operation), nil
	})
	if err != nil {
		t.Fatalf("NewResourceCommand() error = %v", err)
	}

	if findDirectCommand(command, "list") == nil {
		t.Fatalf("missing list command")
	}
	if findDirectCommand(command, "messages") == nil {
		t.Fatalf("missing nested messages command")
	}
}

func TestAddShortcutsRegistersSiblingCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "metorial-admin"}
	spec := ResourceSpec{
		Plural:   "integrations",
		Singular: "integrations",
		Short:    "Integrations",
		Children: []ResourceSpec{
			{
				Plural:   "setup-sessions",
				Singular: "setup-sessions",
				Short:    "Setup sessions",
				Operations: []OperationSpec{
					{Name: OperationCreate, Method: "POST", Path: "/integration-setup-sessions", Short: "Create setup session"},
				},
				Shortcuts: []ShortcutSpec{
					{Path: []string{"integrations", "setup"}, Method: OperationCreate},
				},
			},
		},
	}

	command, err := NewResourceCommand(spec, func(resource ResourceSpec, operation OperationSpec) (*cobra.Command, error) {
		return NewPlaceholderAction(resource, operation), nil
	})
	if err != nil {
		t.Fatalf("NewResourceCommand() error = %v", err)
	}
	root.AddCommand(command)

	if err := AddShortcuts(root, spec, func(resource ResourceSpec, operation OperationSpec) (*cobra.Command, error) {
		return NewPlaceholderAction(resource, operation), nil
	}); err != nil {
		t.Fatalf("AddShortcuts() error = %v", err)
	}

	if CommandByName(command, "setup") == nil {
		t.Fatal("missing integrations setup shortcut")
	}
}
