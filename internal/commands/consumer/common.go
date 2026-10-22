package consumer

import (
	"fmt"
	"io"
	"strings"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

const defaultPageLimit = 15

type commandEnv struct {
	ctx commandutil.Context
}

func (e commandEnv) app() *app.App {
	return e.ctx.App
}

func (e commandEnv) format() string {
	return e.ctx.Options.Format
}

func (e commandEnv) client() (*consumerapi.Client, error) {
	runtime, err := e.ctx.ResolveRuntime()
	if err != nil {
		return nil, err
	}

	return consumerapi.New(runtime)
}

type paginationOptions struct {
	Limit  float64
	After  string
	Before string
	Cursor string
}

func (o paginationOptions) list() consumerapi.ListOptions {
	return consumerapi.ListOptions{
		Limit:  o.Limit,
		After:  o.After,
		Before: o.Before,
		Cursor: o.Cursor,
	}
}

func addPaginationFlags(command *cobra.Command, options *paginationOptions, label string) {
	command.Flags().Float64Var(&options.Limit, "limit", options.Limit, "Limit the number of "+label)
	command.Flags().StringVar(&options.After, "after", "", "Fetch "+label+" after this cursor")
	command.Flags().StringVar(&options.Before, "before", "", "Fetch "+label+" before this cursor")
	command.Flags().StringVar(&options.Cursor, "cursor", "", "Fetch "+label+" using an opaque cursor")
}

func paginationTips(base string, options paginationOptions, firstID, lastID string, pagination consumerapi.Pagination) []string {
	if firstID == "" && lastID == "" {
		return nil
	}

	if options.Limit > 0 && options.Limit != defaultPageLimit {
		base += fmt.Sprintf(" --limit %s", strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", options.Limit), "0"), "."))
	}

	tips := []string{}
	if pagination.HasMoreAfter {
		tips = append(tips, fmt.Sprintf("%s --after %s", base, commandutil.ShellQuote(lastID)))
	}
	if pagination.HasMoreBefore {
		tips = append(tips, fmt.Sprintf("%s --before %s", base, commandutil.ShellQuote(firstID)))
	}
	return tips
}

func renderServerEndpoints(writer io.Writer, features terminal.Features, server *consumerapi.MagicMcpServer) error {
	if len(server.Endpoints) == 0 {
		return nil
	}

	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Accent("Endpoints"))

	table := output.Table{
		Columns:  []string{colors.Accent("Alias"), colors.Accent("URL")},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, endpoint := range server.Endpoints {
		table.Rows = append(table.Rows, []string{endpoint.Alias, endpoint.URL})
	}
	return table.Render(writer)
}

func primaryServerURL(server *consumerapi.MagicMcpServer) string {
	if server == nil || len(server.Endpoints) == 0 {
		return ""
	}
	return server.Endpoints[0].URL
}

func integrationIdentifier(server *consumerapi.MagicMcpServer) string {
	if server == nil {
		return ""
	}
	if len(server.Endpoints) > 0 && strings.TrimSpace(server.Endpoints[0].Alias) != "" {
		return strings.TrimSpace(server.Endpoints[0].Alias)
	}
	return server.ID
}

func integrationTitle(server *consumerapi.MagicMcpServer) string {
	return commandutil.FirstNonEmpty(commandutil.OptionalString(server.Name), integrationIdentifier(server))
}

func integrationTips(server *consumerapi.MagicMcpServer, includeInstall bool) []string {
	identifier := integrationIdentifier(server)
	tips := []string{
		fmt.Sprintf("metorial tools %s", identifier),
	}
	if includeInstall {
		tips = append(tips,
			fmt.Sprintf("metorial install codex %s", identifier),
			fmt.Sprintf("metorial install custom %s", identifier),
		)
	}
	return tips
}

func stringPtrIfSet(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
