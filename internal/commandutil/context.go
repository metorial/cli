package commandutil

import (
	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
)

type RootOptions struct {
	APIKey   string
	APIHost  string
	Instance string
	Profile  string
	Format   string
}

type Context struct {
	App     *app.App
	Options *RootOptions
}

func NewContext(application *app.App, options *RootOptions) Context {
	return Context{
		App:     application,
		Options: options,
	}
}

func (c Context) ResolveRuntime() (config.Runtime, error) {
	return c.App.ResolveConfig(c.Options.APIKey, c.Options.APIHost, c.Options.Profile, c.Options.Instance)
}

func (c Context) ResolveAuthRuntime() (config.Runtime, error) {
	return c.App.ResolveAuthConfig(c.Options.APIKey, c.Options.APIHost, c.Options.Profile)
}
