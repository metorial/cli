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
	}

	for _, operation := range resource.Operations {
		actionCommand, err := builder(resource, operation)
		if err != nil {
			return nil, fmt.Errorf("resource command %q: %w", resource.Plural, err)
		}
		command.AddCommand(actionCommand)
	}

	return command, nil
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
