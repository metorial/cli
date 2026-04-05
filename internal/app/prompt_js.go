//go:build js

package app

import (
	"fmt"

	instancesresource "github.com/metorial/metorial-go/v1/resources/instances"
)

func (a *App) promptForInstance(instances []instancesresource.InstancesListOutputItems) (*instancesresource.InstancesListOutputItems, error) {
	return nil, fmt.Errorf("metorial: interactive instance selection is not available in the browser shell. Use --instance <instance-id> instead")
}
