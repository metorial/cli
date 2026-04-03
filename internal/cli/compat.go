package cli

import (
	"github.com/metorial/cli/internal/app"
	settingscmd "github.com/metorial/cli/internal/commands/settings"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	apiKey   string
	apiHost  string
	instance string
	profile  string
	format   string
}

func (o *rootOptions) toRootOptions() *commandutil.RootOptions {
	if o == nil {
		return &commandutil.RootOptions{}
	}

	return &commandutil.RootOptions{
		APIKey:   o.apiKey,
		APIHost:  o.apiHost,
		Instance: o.instance,
		Profile:  o.profile,
		Format:   o.format,
	}
}

func newSettingsCommand(application *app.App, options *rootOptions) *cobra.Command {
	return settingscmd.NewCommand(commandutil.NewContext(application, options.toRootOptions()))
}
