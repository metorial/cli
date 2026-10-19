package consumer

import (
	"fmt"
	"io"
	"strings"

	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

type integrationListRow struct {
	ID          string `json:"id"`
	Alias       string `json:"alias,omitempty"`
	Status      string `json:"status"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	EndpointURL string `json:"endpoint_url,omitempty"`
}

func newIntegrationCommands(env commandEnv) []*cobra.Command {
	return []*cobra.Command{
		newListCommand(env),
		newGetCommand(env),
		newUpdateCommand(env),
		newRemoveCommand(env),
	}
}

func newListCommand(env commandEnv) *cobra.Command {
	pagination := paginationOptions{Limit: defaultPageLimit}

	command := &cobra.Command{
		Use:   "list [search]",
		Short: "List your integrations, optionally filtering by a search term",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			search := strings.TrimSpace(optionalArg(args, 0))
			result, err := client.ListMagicMcpServers(pagination.list(), search, "")
			if err != nil {
				return err
			}

			rows := make([]integrationListRow, 0, len(result.Items))
			for index := range result.Items {
				server := &result.Items[index]
				rows = append(rows, integrationListRow{
					ID:          server.ID,
					Alias:       integrationIdentifier(server),
					Status:      server.Status,
					Name:        optionalString(server.Name),
					Description: optionalString(server.Description),
					EndpointURL: primaryServerURL(server),
				})
			}

			tips := []string{}
			firstID, lastID := "", ""
			if len(result.Items) > 0 {
				first := &result.Items[0]
				firstID, lastID = first.ID, result.Items[len(result.Items)-1].ID
				tips = append(tips, fmt.Sprintf("metorial get %s", integrationIdentifier(first)))
				tips = append(tips, integrationTips(first, false)...)
			}
			base := "metorial list"
			if search != "" {
				base += " " + shellQuote(search)
			}
			tips = append(tips, paginationTips(base, pagination, firstID, lastID, result.Pagination)...)
			tips = append(tips, "metorial catalog list")

			return env.write(command, map[string]any{
				"items":      result.Items,
				"rows":       rows,
				"pagination": result.Pagination,
				"tips":       tips,
			}, func(writer io.Writer, features terminal.Features) error {
				return renderIntegrationList(writer, features, rows, tips)
			})
		},
	}
	addPaginationFlags(command, &pagination, "integrations")
	return command
}

func newGetCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "get <integration-id>",
		Short: "View an integration's details",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			server, err := client.GetMagicMcpServer(args[0])
			if err != nil {
				return err
			}

			tips := integrationTips(server, true)

			return env.write(command, map[string]any{
				"integration": server,
				"tips":        tips,
			}, func(writer io.Writer, features terminal.Features) error {
				return renderIntegrationDetail(writer, features, server, tips)
			})
		},
	}
}

func newUpdateCommand(env commandEnv) *cobra.Command {
	name := ""
	description := ""

	command := &cobra.Command{
		Use:   "update <integration-id>",
		Short: "Rename an integration or change its description",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			input := consumerapi.UpdateMagicMcpServerInput{}
			if command.Flags().Changed("name") {
				input.Name = &name
			}
			if command.Flags().Changed("description") {
				input.Description = &description
			}
			if input.Name == nil && input.Description == nil {
				return fmt.Errorf("metorial: nothing to update. Pass --name and/or --description")
			}

			client, err := env.client()
			if err != nil {
				return err
			}

			server, err := client.GetMagicMcpServer(args[0])
			if err != nil {
				return err
			}

			updated, err := client.UpdateMagicMcpServer(server.ID, input)
			if err != nil {
				return err
			}

			tips := []string{fmt.Sprintf("metorial get %s", integrationIdentifier(updated))}

			return env.write(command, map[string]any{
				"integration": updated,
				"tips":        tips,
			}, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintln(writer, colors.Success("Integration Updated"))
				_, _ = fmt.Fprintln(writer)
				return renderIntegrationDetail(writer, features, updated, tips)
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "New name for the integration")
	command.Flags().StringVar(&description, "description", "", "New description for the integration")
	return command
}

func newRemoveCommand(env commandEnv) *cobra.Command {
	yes := false

	command := &cobra.Command{
		Use:     "remove <integration-id>",
		Aliases: []string{"delete"},
		Short:   "Delete one of your integrations",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			server, err := client.GetMagicMcpServer(args[0])
			if err != nil {
				return err
			}

			if !yes {
				if !env.app().StdoutFeatures().IsTTY {
					return fmt.Errorf("metorial: refusing to delete %s without confirmation. Pass --yes to confirm", integrationIdentifier(server))
				}
				if !env.app().PromptConfirm(fmt.Sprintf("Delete %s? Its endpoint URL and tokens will stop working", integrationTitle(server))) {
					return fmt.Errorf("metorial: deletion cancelled")
				}
			}

			deleted, err := client.DeleteMagicMcpServer(server.ID)
			if err != nil {
				return err
			}

			return env.write(command, map[string]any{
				"integration": deleted,
			}, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintln(writer, colors.Success("Integration Deleted"))
				_, _ = fmt.Fprintln(writer)
				_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Muted("Removed:"), colors.Bold(integrationTitle(server)))
				return nil
			})
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "Delete without asking for confirmation")
	return command
}

func renderIntegrationList(writer io.Writer, features terminal.Features, rows []integrationListRow, tips []string) error {
	colors := terminal.NewColorizer(features)
	renderTitle(writer, features, "Your Integrations", "")

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(writer, "(no integrations)")
		return renderTips(writer, features, tips)
	}

	table := output.Table{
		Columns: []string{
			colors.Accent("Name"),
			colors.Accent("Status"),
			colors.Accent("Endpoint"),
			colors.Accent("Description"),
		},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, row := range rows {
		name := row.Alias
		if row.Name != "" {
			name = colors.Bold(row.Name) + "\n" + colors.Muted(row.Alias)
		}
		table.Rows = append(table.Rows, []string{name + "\n", row.Status, row.EndpointURL, row.Description})
	}
	if err := table.Render(writer); err != nil {
		return err
	}

	return renderTips(writer, features, tips)
}

func renderIntegrationDetail(writer io.Writer, features terminal.Features, server *consumerapi.MagicMcpServer, tips []string) error {
	colors := terminal.NewColorizer(features)
	renderTitle(writer, features, integrationTitle(server), server.ID)

	items := []output.DataListItem{
		{Label: "Status", Value: server.Status},
	}
	if description := optionalString(server.Description); description != "" {
		items = append(items, output.DataListItem{Label: "Description", Value: description})
	}
	if err := output.RenderDataList(writer, items); err != nil {
		return err
	}

	if err := renderServerEndpoints(writer, features, server); err != nil {
		return err
	}

	if len(server.Providers) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Providers"))
		table := output.Table{
			Columns:  []string{colors.Accent("Name"), colors.Accent("Status")},
			Features: features,
			MaxWidth: features.Width,
		}
		for _, provider := range server.Providers {
			name := provider.Name
			if provider.Provider != nil {
				name = firstNonEmpty(provider.Provider.Name, provider.Name)
			}
			table.Rows = append(table.Rows, []string{name, provider.Status})
		}
		if err := table.Render(writer); err != nil {
			return err
		}
	}

	return renderTips(writer, features, tips)
}
