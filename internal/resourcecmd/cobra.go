package resourcecmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type OperationBuilder func(resource ResourceSpec, operation OperationSpec) (*cobra.Command, error)

func NewResourceCommand(resource ResourceSpec, builder OperationBuilder) (*cobra.Command, error) {
	if err := resource.Validate(); err != nil {
		return nil, err
	}

	command := &cobra.Command{
		Use:     resource.DefaultUse(),
		Aliases: resource.CobraAliases(),
		Short:   resource.Short,
		Long:    resource.Long,
	}

	for _, operation := range resource.Operations {
		actionCommand, err := builder(resource, operation)
		if err != nil {
			return nil, fmt.Errorf("resource command %q: %w", resource.Plural, err)
		}
		command.AddCommand(actionCommand)
	}

	for _, child := range resource.Children {
		childCommand, err := NewResourceCommand(child, builder)
		if err != nil {
			return nil, err
		}
		command.AddCommand(childCommand)
	}

	return command, nil
}

func AddShortcuts(root *cobra.Command, resource ResourceSpec, builder OperationBuilder) error {
	for _, shortcut := range resource.Shortcuts {
		operation, ok := resource.Operation(shortcut.Method)
		if !ok {
			return fmt.Errorf("resource command %q: shortcut method %q not found", resource.Plural, shortcut.Method)
		}

		parent := root
		for _, part := range shortcut.Path[:len(shortcut.Path)-1] {
			found := findDirectCommand(parent, part)
			if found == nil {
				return fmt.Errorf("resource command %q: shortcut parent %q not found", resource.Plural, strings.Join(shortcut.Path, " "))
			}
			parent = found
		}

		shortcutOperation := operation
		shortcutOperation.Name = OperationName(shortcut.Path[len(shortcut.Path)-1])
		shortcutOperation.Use = defaultOperationUse(shortcutOperation)
		if shortcutOperation.Short == "" {
			shortcutOperation.Short = operation.Short
		}

		action, err := builder(resource, shortcutOperation)
		if err != nil {
			return fmt.Errorf("resource command %q: shortcut %q: %w", resource.Plural, strings.Join(shortcut.Path, " "), err)
		}
		parent.AddCommand(action)
	}

	for _, child := range resource.Children {
		if err := AddShortcuts(root, child, builder); err != nil {
			return err
		}
	}

	return nil
}

func CommandByName(parent *cobra.Command, name string) *cobra.Command {
	return findDirectCommand(parent, name)
}

func findDirectCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
		if command.Name() == name {
			return command
		}
		for _, alias := range command.Aliases {
			if alias == name {
				return command
			}
		}
	}
	return nil
}

func NewPlaceholderAction(resource ResourceSpec, operation OperationSpec) *cobra.Command {
	use := operation.Use
	if use == "" {
		use = defaultOperationUse(operation)
	}

	short := operation.Short
	if short == "" {
		short = fmt.Sprintf("%s %s", operation.Name, resource.Plural)
	}

	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(command *cobra.Command, args []string) error {
			return fmt.Errorf("metorial: %s %s is not implemented yet", resource.Plural, operation.Name)
		},
	}
}

func defaultOperationUse(operation OperationSpec) string {
	parts := []string{string(operation.Name)}
	for _, arg := range operation.Args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if arg.Required {
			parts = append(parts, "<"+name+">")
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	return strings.Join(parts, " ")
}
