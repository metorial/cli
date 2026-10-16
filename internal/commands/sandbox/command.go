package sandbox

import (
	"errors"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/metorial/cli/internal/sandboxaccess"
	"github.com/spf13/cobra"
)

// Attach adds the hand-written shell and files commands under the generated
// `sandbox machines` command, so it must run after the generated commands exist.
func Attach(root *cobra.Command, ctx commandutil.Context) error {
	sandbox := resourcecmd.CommandByName(root, "sandbox")
	if sandbox == nil {
		return errors.New("metorial: the generated sandbox command is missing")
	}
	machines := resourcecmd.CommandByName(sandbox, "machines")
	if machines == nil {
		return errors.New("metorial: the generated sandbox machines command is missing")
	}

	for _, command := range []*cobra.Command{newShellCommand(ctx), newFilesCommand(ctx)} {
		configure(command)
		machines.AddCommand(command)
	}
	return nil
}

func configure(command *cobra.Command) {
	commandutil.ConfigureCommand(command)
	commandutil.SetCommandCategory(command, commandutil.CommandCategoryResource)
	for _, child := range command.Commands() {
		configure(child)
	}
}

func newSource(ctx commandutil.Context, machineID string, scope sandboxaccess.Scope, ttlSeconds int) (*sandboxaccess.Source, error) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return nil, errors.New("metorial: a sandbox machine id is required")
	}

	runtime, err := ctx.ResolveRuntime()
	if err != nil {
		return nil, err
	}
	if err := runtime.RequireAPIKey(); err != nil {
		return nil, err
	}

	return sandboxaccess.NewSource(sandboxaccess.APIMinter(runtime, machineID, scope, ttlSeconds)), nil
}
