package fetch

import (
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCommand(ctx commandutil.Context) *cobra.Command {
	options := &fetch.Options{}

	command := &cobra.Command{
		Use:     "fetch <path-or-url>",
		Aliases: []string{"curl"},
		Short:   "Send an authenticated request to the Metorial API",
		Long: strings.TrimSpace(`
Send a raw HTTP request to the selected Metorial API host.

The request inherits authentication from --api-key, METORIAL_API_KEY,
METORIAL_TOKEN, or the current OAuth profile. When the current profile has a
refresh token, the CLI refreshes it automatically before requests when needed.
Use --profile to override the selected saved profile for this command. Use the
global --format flag to switch between YAML, TOML, JSON, and structured output.

Examples:
  metorial fetch /provider-listings
  metorial fetch /provider-listings -H 'X-Debug: true'
  metorial fetch /provider-listings -X POST -d '{"name":"demo"}'
  metorial curl https://api.metorial.com/provider-listings -i
`),
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := ctx.ResolveRuntime()
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(ctx.Options.Format)
			if err != nil {
				return err
			}

			options.Target = args[0]

			response, requestErr := fetch.Execute(runtime, *options, ctx.App.Stdin)
			if response != nil {
				writer := command.OutOrStdout()
				if requestErr != nil {
					writer = command.ErrOrStderr()
				}
				if err := output.WriteResponse(writer, response, output.RenderOptions{
					Format:  format,
					Include: options.Include,
					Colors:  ctx.App.StdoutFeatures(),
				}); err != nil {
					return err
				}
			}

			return requestErr
		},
	}

	command.Flags().StringSliceVarP(&options.Headers, "header", "H", nil, "Add request header in the form KEY: VALUE")
	command.Flags().StringVarP(&options.Method, "method", "X", "", "HTTP method to use")
	command.Flags().StringVarP(&options.Data, "data", "d", "", "Request body data, or @- to read from stdin")
	command.Flags().StringVar(&options.BodyFile, "body-file", "", "Read the request body from a file, or - for stdin")
	command.Flags().BoolVarP(&options.Include, "include", "i", false, "Include the response status line and headers in the output")

	return command
}
