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

func (e commandEnv) structured() (bool, error) {
	format, err := output.ParseFormat(e.format())
	if err != nil {
		return false, err
	}

	return format == output.FormatStructured, nil
}

func (e commandEnv) write(command *cobra.Command, value any, render func(io.Writer, terminal.Features) error) error {
	structured, err := e.structured()
	if err != nil {
		return err
	}
	if !structured {
		return commandutil.WriteValue(command.OutOrStdout(), e.app().StdoutFeatures(), e.format(), value)
	}

	return render(command.OutOrStdout(), e.app().StdoutFeatures())
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
		tips = append(tips, fmt.Sprintf("%s --after %s", base, shellQuote(lastID)))
	}
	if pagination.HasMoreBefore {
		tips = append(tips, fmt.Sprintf("%s --before %s", base, shellQuote(firstID)))
	}
	return tips
}

func renderTips(writer io.Writer, features terminal.Features, tips []string) error {
	if len(tips) == 0 {
		return nil
	}

	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Notice("Tips"))
	for _, tip := range tips {
		if strings.TrimSpace(tip) == "" {
			continue
		}
		_, _ = fmt.Fprintf(writer, "  %s\n", tip)
	}
	return nil
}

func renderTitle(writer io.Writer, features terminal.Features, title, subtitle string) {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Bold(title))
	if strings.TrimSpace(subtitle) != "" {
		_, _ = fmt.Fprintln(writer, colors.Muted(subtitle))
	}
	_, _ = fmt.Fprintln(writer)
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
	return firstNonEmpty(optionalString(server.Name), integrationIdentifier(server))
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

func optionalArg(args []string, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	return args[index]
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPtrIfSet(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	return commandutil.FirstNonEmpty(values...)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
