package resources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/metorial/cli/internal/apiclient"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/spf13/cobra"
)

func newProviderAuthCommand(cli commandutil.Context) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth <provider-id>",
		Short: "Show a provider's auth methods and available credentials",
		Long: "Inspect the provider's current auth methods and available managed or custom credentials. " +
			"Pass the selected method as --provider-auth-method-id and a compatible OAuth app as --provider-auth-credentials-id. " +
			"OAuth auto-registration describes creating a new app.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			format, err := output.ParseFormat(cli.Options.Format)
			if err != nil {
				return err
			}
			runtime, err := cli.ResolveRuntime()
			if err != nil {
				return err
			}

			client := apiclient.Client{Runtime: runtime}
			var provider struct {
				ID             string          `json:"id"`
				Name           string          `json:"name"`
				OAuth          json.RawMessage `json:"oauth"`
				CurrentVersion struct {
					ID string `json:"id"`
				} `json:"current_version"`
			}
			if err := client.Do(http.MethodGet, "/providers/"+apiclient.PathSegment(args[0]), nil, &provider); err != nil {
				return err
			}
			if provider.ID == "" {
				return fmt.Errorf("metorial: provider response is missing an ID")
			}

			var methods, credentials json.RawMessage
			if provider.CurrentVersion.ID != "" {
				query := url.Values{"provider_version_id": {provider.CurrentVersion.ID}}
				if err := client.Do(http.MethodGet, "/provider-auth-methods?"+query.Encode(), nil, &methods); err != nil {
					return err
				}
			}
			query := url.Values{"provider_id": {provider.ID}}
			if err := client.Do(http.MethodGet, "/provider-auth-credentials?"+query.Encode(), nil, &credentials); err != nil {
				return err
			}

			body, err := json.Marshal(map[string]any{
				"provider_id":         provider.ID,
				"name":                provider.Name,
				"provider_version_id": provider.CurrentVersion.ID,
				"oauth":               provider.OAuth,
				"auth_methods":        methods,
				"auth_credentials":    credentials,
			})
			if err != nil {
				return err
			}
			return output.WriteResponse(command.OutOrStdout(), &fetch.Response{Body: body}, output.RenderOptions{
				Format: format, Colors: cli.App.StdoutFeatures(),
			})
		},
	}
	commandutil.ConfigureCommand(command)
	return command
}
