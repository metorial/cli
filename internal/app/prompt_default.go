//go:build !js

package app

import (
	"fmt"

	"github.com/manifoldco/promptui"
	instancesresource "github.com/metorial/metorial-go/v1/resources/instances"
)

func (a *App) promptForInstance(instances []instancesresource.InstancesListOutputItems) (*instancesresource.InstancesListOutputItems, error) {
	items := make([]string, 0, len(instances))
	for _, instance := range instances {
		items = append(items, fmt.Sprintf("%s (%s)", firstNonEmpty(instance.Name, instance.Slug), instance.Id))
	}

	prompt := promptui.Select{
		Label: "Select an instance",
		Items: items,
		Stdin: readerCloser{
			Reader: a.Stdin,
		},
		Stdout: writerCloser{
			Writer: a.Stdout,
		},
	}

	index, _, err := prompt.Run()
	if err != nil {
		return nil, fmt.Errorf("metorial: instance selection was cancelled. Use --instance <instance-id> or run \"metorial instances list\" to see available instances")
	}

	selected := instances[index]
	return &selected, nil
}
