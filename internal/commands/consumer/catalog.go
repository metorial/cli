package consumer

import (
	"fmt"
	"io"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

type catalogListRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Availability string `json:"availability"`
	Description  string `json:"description,omitempty"`
}

func newCatalogCommands(env commandEnv) []*cobra.Command {
	pagination := paginationOptions{Limit: defaultPageLimit}
	groupID := ""

	runList := func(command *cobra.Command, args []string) error {
		client, err := env.client()
		if err != nil {
			return err
		}

		search := strings.TrimSpace(commandutil.OptionalArg(args, 0))
		result, err := client.ListProviders(pagination.list(), search, groupID)
		if err != nil {
			return err
		}

		rows := make([]catalogListRow, 0, len(result.Items))
		for _, item := range result.Items {
			rows = append(rows, catalogListRow{
				ID:           item.ID,
				Name:         item.Name,
				Type:         catalogTypeLabel(item),
				Availability: availabilityLabel(item),
				Description:  commandutil.OptionalString(item.Description),
			})
		}

		tips := []string{}
		firstID, lastID := "", ""
		if len(result.Items) > 0 {
			first := result.Items[0]
			firstID, lastID = first.ID, result.Items[len(result.Items)-1].ID
			tips = append(tips, fmt.Sprintf("metorial catalog get %s", first.ID))
			tips = append(tips, catalogNextStep(first))
		}

		base := command.CommandPath()
		if search != "" {
			base += " " + commandutil.ShellQuote(search)
		}
		if groupID != "" {
			base += " --group " + commandutil.ShellQuote(groupID)
		}
		tips = append(tips, paginationTips(base, pagination, firstID, lastID, result.Pagination)...)

		return env.ctx.Write(command, map[string]any{
			"items":      result.Items,
			"rows":       rows,
			"pagination": result.Pagination,
			"tips":       tips,
		}, func(writer io.Writer, features terminal.Features) error {
			return renderCatalogList(writer, features, rows, tips)
		})
	}

	newListCommand := func(use, short string) *cobra.Command {
		command := &cobra.Command{
			Use:   use + " [search]",
			Short: short,
			Args:  cobra.RangeArgs(0, 1),
			RunE:  runList,
		}
		addPaginationFlags(command, &pagination, "catalog items")
		command.Flags().StringVar(&groupID, "group", "", "Only show catalog items in this provider group")
		return command
	}

	catalogCommand := &cobra.Command{
		Use:   "catalog",
		Short: "Browse the integrations available in your portal",
	}
	catalogCommand.AddCommand(newListCommand("list", "List the integrations available in your portal"))
	catalogCommand.AddCommand(newListCommand("search", "Search the integrations available in your portal"))
	catalogCommand.AddCommand(newCatalogGroupsCommand(env))
	catalogCommand.AddCommand(newCatalogGetCommand(env))

	return []*cobra.Command{
		catalogCommand,
		newListCommand("search", "Search the integrations available in your portal"),
		newRequestAccessCommand(env),
	}
}

func newCatalogGroupsCommand(env commandEnv) *cobra.Command {
	pagination := paginationOptions{Limit: 50}

	command := &cobra.Command{
		Use:   "groups",
		Short: "List the provider groups of your portal catalog",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			result, err := client.ListProviderGroups(pagination.list())
			if err != nil {
				return err
			}

			tips := []string{}
			if len(result.Items) > 0 {
				tips = append(tips, fmt.Sprintf("metorial catalog list --group %s", result.Items[0].ID))
			}

			return env.ctx.Write(command, map[string]any{
				"items":      result.Items,
				"pagination": result.Pagination,
				"tips":       tips,
			}, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				commandutil.RenderTitle(writer, features, "Catalog Groups", "Groups your portal uses to organize integrations.")

				if len(result.Items) == 0 {
					_, _ = fmt.Fprintln(writer, "(no groups)")
					return commandutil.RenderTips(writer, features, tips)
				}

				table := output.Table{
					Columns:  []string{colors.Accent("Group"), colors.Accent("Description")},
					Features: features,
					MaxWidth: features.Width,
				}
				for _, group := range result.Items {
					table.Rows = append(table.Rows, []string{
						colors.Bold(group.Name) + "\n" + colors.Muted(group.ID) + "\n",
						commandutil.OptionalString(group.Description),
					})
				}
				if err := table.Render(writer); err != nil {
					return err
				}
				return commandutil.RenderTips(writer, features, tips)
			})
		},
	}
	addPaginationFlags(command, &pagination, "groups")
	return command
}

func newCatalogGetCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "get <catalog-item-id>",
		Short: "Show a catalog item, its auth methods, and its tools",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			item, err := client.GetProvider(args[0])
			if err != nil {
				return err
			}

			tools := []consumerapi.SpecificationTool{}
			if specificationID := catalogSpecificationID(item); specificationID != "" {
				specification, err := client.GetProviderSpecification(specificationID)
				if err == nil {
					tools = specification.Tools
				}
			}

			owned := []consumerapi.MagicMcpServer{}
			if item.ProviderTemplate != nil {
				servers, err := client.ListMagicMcpServers(consumerapi.ListOptions{Limit: 50}, "", item.ProviderTemplate.ID)
				if err != nil {
					return err
				}
				owned = servers.Items
			}

			tips := []string{catalogNextStep(*item)}
			if len(owned) > 0 {
				tips = append(tips, fmt.Sprintf("metorial tools %s", integrationIdentifier(&owned[0])))
				tips = append(tips, fmt.Sprintf("metorial reconnect %s %s", item.ID, owned[0].ID))
			}

			return env.ctx.Write(command, map[string]any{
				"item":         item,
				"tools":        tools,
				"integrations": owned,
				"tips":         tips,
			}, func(writer io.Writer, features terminal.Features) error {
				return renderCatalogDetail(writer, features, item, tools, owned, tips)
			})
		},
	}
}

func newRequestAccessCommand(env commandEnv) *cobra.Command {
	message := ""

	command := &cobra.Command{
		Use:   "request-access <catalog-item-id>",
		Short: "Ask your portal administrators for access to an integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			item, err := client.GetProvider(args[0])
			if err != nil {
				return err
			}
			if item.Availability != consumerapi.AvailabilityRequestAccess {
				return fmt.Errorf("metorial: %s is already available to you. Run `%s` to set it up", item.Name, catalogNextStep(*item))
			}
			if item.HasPendingAccessRequest {
				return fmt.Errorf("metorial: you already requested access to %s. Your portal administrators will review it", item.Name)
			}

			request, err := client.RequestAccess(item.ID, message)
			if err != nil {
				return err
			}

			return env.ctx.Write(command, map[string]any{
				"item":           item,
				"access_request": request,
			}, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintln(writer, colors.Success("Access Requested"))
				_, _ = fmt.Fprintln(writer)
				if err := output.RenderDataList(writer, []output.DataListItem{
					{Label: "Integration", Value: item.Name},
					{Label: "Request", Value: request.ID},
					{Label: "Status", Value: request.Status},
				}); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(writer)
				_, _ = fmt.Fprintln(writer, colors.Muted("Your portal administrators will review the request."))
				return nil
			})
		},
	}
	command.Flags().StringVarP(&message, "message", "m", "", "Optional message for the administrators reviewing the request")
	return command
}

func renderCatalogList(writer io.Writer, features terminal.Features, rows []catalogListRow, tips []string) error {
	colors := terminal.NewColorizer(features)
	commandutil.RenderTitle(writer, features, "Integration Catalog", "Integrations your portal makes available to you.")

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(writer, "(no integrations)")
		return commandutil.RenderTips(writer, features, tips)
	}

	table := output.Table{
		Columns: []string{
			colors.Accent("Name"),
			colors.Accent("Availability"),
			colors.Accent("Description"),
		},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, row := range rows {
		table.Rows = append(table.Rows, []string{
			colors.Bold(row.Name) + "\n" + colors.Muted(row.ID) + "\n",
			row.Availability + "\n" + colors.Muted(row.Type),
			row.Description,
		})
	}
	if err := table.Render(writer); err != nil {
		return err
	}

	return commandutil.RenderTips(writer, features, tips)
}

func renderCatalogDetail(writer io.Writer, features terminal.Features, item *consumerapi.Provider, tools []consumerapi.SpecificationTool, owned []consumerapi.MagicMcpServer, tips []string) error {
	colors := terminal.NewColorizer(features)
	commandutil.RenderTitle(writer, features, item.Name, item.ID)

	items := []output.DataListItem{
		{Label: "Type", Value: catalogTypeLabel(*item)},
		{Label: "Availability", Value: availabilityLabel(*item)},
	}
	if item.Provider != nil {
		provider := item.Provider.Name
		if item.Provider.Publisher != nil && item.Provider.Publisher.Name != "" {
			provider += " by " + item.Provider.Publisher.Name
		}
		items = append(items, output.DataListItem{Label: "Provider", Value: provider})
	}
	if description := commandutil.OptionalString(item.Description); description != "" {
		items = append(items, output.DataListItem{Label: "Description", Value: description})
	}
	if err := output.RenderDataList(writer, items); err != nil {
		return err
	}

	if len(item.AuthMethods) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Auth Methods"))
		table := output.Table{
			Columns:  []string{colors.Accent("Name"), colors.Accent("Type"), colors.Accent("ID")},
			Features: features,
			MaxWidth: features.Width,
		}
		for _, method := range item.AuthMethods {
			table.Rows = append(table.Rows, []string{method.Name, method.Type, method.ID})
		}
		if err := table.Render(writer); err != nil {
			return err
		}
	}

	if len(owned) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Your Integrations"))
		table := output.Table{
			Columns:  []string{colors.Accent("Name"), colors.Accent("Status"), colors.Accent("Endpoint")},
			Features: features,
			MaxWidth: features.Width,
		}
		for index := range owned {
			server := &owned[index]
			table.Rows = append(table.Rows, []string{
				colors.Bold(integrationTitle(server)) + "\n" + colors.Muted(server.ID),
				server.Status,
				primaryServerURL(server),
			})
		}
		if err := table.Render(writer); err != nil {
			return err
		}
	}

	if readme := commandutil.OptionalString(item.Readme); readme != "" {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Readme"))
		_, _ = fmt.Fprintln(writer, readme)
	}

	if len(tools) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Tools"))
		table := output.Table{
			Columns:  []string{colors.Accent("Key"), colors.Accent("Name"), colors.Accent("Description")},
			Features: features,
			MaxWidth: features.Width,
		}
		for _, tool := range tools {
			table.Rows = append(table.Rows, []string{colors.Bold(tool.Key), tool.Name, commandutil.OptionalString(tool.Description)})
		}
		if err := table.Render(writer); err != nil {
			return err
		}
	}

	return commandutil.RenderTips(writer, features, tips)
}

func catalogSpecificationID(item *consumerapi.Provider) string {
	if item.Provider == nil || item.Provider.CurrentVersion == nil {
		return ""
	}
	return commandutil.OptionalString(item.Provider.CurrentVersion.SpecificationID)
}

func catalogTypeLabel(item consumerapi.Provider) string {
	if item.Type == consumerapi.ProviderTypeMagicMcpServer {
		return "shared integration"
	}
	return "configurable"
}

func availabilityLabel(item consumerapi.Provider) string {
	switch {
	case item.Availability == consumerapi.AvailabilityAvailableNow:
		return "available"
	case item.HasPendingAccessRequest:
		return "access requested"
	default:
		return "request access"
	}
}

func catalogNextStep(item consumerapi.Provider) string {
	switch {
	case item.Availability == consumerapi.AvailabilityRequestAccess:
		return fmt.Sprintf("metorial request-access %s", item.ID)
	case item.Type == consumerapi.ProviderTypeMagicMcpServer && item.MagicMcpServer != nil:
		return fmt.Sprintf("metorial tools %s", item.MagicMcpServer.ID)
	default:
		return fmt.Sprintf("metorial setup %s", item.ID)
	}
}
