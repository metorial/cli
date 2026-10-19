//go:build js

package app

import (
	"fmt"

	instancesresource "github.com/metorial/metorial-go/v1/resources/instances"
)

func (a *App) promptForInstance(instances []instancesresource.InstancesListOutputItems) (*instancesresource.InstancesListOutputItems, error) {
	return nil, fmt.Errorf("metorial: interactive instance selection is not available in the browser shell. Use --instance <instance-id> instead")
}

func (a *App) PromptSelect(label string, items []string) (int, error) {
	return -1, fmt.Errorf("metorial: interactive selection is not available in the browser shell")
}

func (a *App) PromptConfirm(label string) bool {
	return false
}
