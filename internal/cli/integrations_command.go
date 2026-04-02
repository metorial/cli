package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	metorial "github.com/metorial/metorial-go/v1"
	"github.com/metorial/metorial-go/v1/endpoints"
	"github.com/metorial/metorial-go/v1/resources/consumers"
	"github.com/metorial/metorial-go/v1/resources/magicmcpservers"
	setupsessions "github.com/metorial/metorial-go/v1/resources/providerdeployments/setupsessions"
	providerlistings "github.com/metorial/metorial-go/v1/resources/providerlistings"
	"github.com/metorial/metorial-go/v1/resources/providers"
	providertools "github.com/metorial/metorial-go/v1/resources/providers/tools"
	"github.com/spf13/cobra"
)

type integrationToolsProvider struct {
	Provider *providers.ProvidersGetOutput                 `json:"provider,omitempty"`
	Listing  *providerlistings.ProviderListingsGetOutput   `json:"listing,omitempty"`
	Tools    []providertools.ProvidersToolsListOutputItems `json:"tools"`
}

type integrationListRow struct {
	Id          string `json:"id"`
	Status      string `json:"status"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	EndpointUrl string `json:"endpoint_url,omitempty"`
}

type catalogListRow struct {
	Id          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Publisher   string `json:"publisher"`
}

func newIntegrationsCommand(application *app.App, rootOptions *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:     "integrations",
		Aliases: []string{"integration"},
		Short:   "Browse and set up consumer-owned integrations",
	}

	command.AddCommand(&cobra.Command{
		Use:   "list [search]",
		Short: "List integrations owned by your consumer",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			sdk, err := runtime.SDK()
			if err != nil {
				return err
			}

			consumer, err := getCLIMemberConsumer(runtime)
			if err != nil {
				return err
			}

			params := &endpoints.MagicMcpServersEndpointListParams{
				Limit:      float64Ptr(100),
				ConsumerId: anyPtr(consumer.Id),
			}
			if search := strings.TrimSpace(optionalArg(args, 0)); search != "" {
				params.Search = &search
			}

			result, err := sdk.MagicMcpServers.List(params)
			if err != nil {
				return err
			}

			rows := make([]integrationListRow, 0, len(result.Items))
			for _, item := range result.Items {
				rows = append(rows, integrationListRow{
					Id:          item.Id,
					Status:      item.Status,
					Name:        optionalString(item.Name),
					Description: optionalString(item.Description),
					EndpointUrl: primaryServerURL(item.Endpoints),
				})
			}

			tips := []string{}
			if len(result.Items) > 0 {
				tips = append(tips, fmt.Sprintf("metorial integrations get %s", result.Items[0].Id))
				tips = append(tips, fmt.Sprintf("metorial integrations tools %s", result.Items[0].Id))
			}
			tips = append(tips, "metorial integrations catalog list")

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"consumer": map[string]any{
						"id":                  consumer.Id,
						"name":                consumer.Name,
						"email":               consumer.Email,
						"consumer_profile_id": consumer.Profile.Id,
					},
					"items":      result.Items,
					"rows":       rows,
					"pagination": result.Pagination,
					"tips":       tips,
				})
			}

			return renderIntegrationsList(command.OutOrStdout(), application.StdoutFeatures(), consumer, rows, tips)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "get <magic-mcp-server-id>",
		Short: "Show a magic MCP server integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			sdk, err := runtime.SDK()
			if err != nil {
				return err
			}

			server, err := sdk.MagicMcpServers.Get(args[0])
			if err != nil {
				return err
			}

			tips := []string{
				fmt.Sprintf("metorial integrations tools %s", server.Id),
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"integration": server,
					"tips":        tips,
				})
			}

			return renderIntegrationDetail(command.OutOrStdout(), application.StdoutFeatures(), server, tips)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "setup [provider-listing-slug-or-id]",
		Short: "Create a new integration through a setup session",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			sdk, err := runtime.SDK()
			if err != nil {
				return err
			}

			consumer, err := getCLIMemberConsumer(runtime)
			if err != nil {
				return err
			}

			var listing *providerlistings.ProviderListingsGetOutput
			if listingID := strings.TrimSpace(optionalArg(args, 0)); listingID != "" {
				listing, err = sdk.ProviderListings.Get(listingID)
				if err != nil {
					return err
				}
			}

			createBody := &endpoints.ProviderDeploymentsSetupSessionsEndpointCreateBody{
				ConsumerId: &consumer.Id,
			}
			if listing != nil {
				createBody.ProviderId = &listing.Provider.Id
			}

			setupSession, err := sdk.ProviderDeploymentsSetupSessions.Create(createBody)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format == output.FormatStructured {
				if err := output.RenderBox(command.OutOrStdout(), []string{
					fmt.Sprintf("Open this URL to continue setup:\n%s", setupSession.Url),
					fmt.Sprintf("Setup session: %s", setupSession.Id),
					fmt.Sprintf("Expires: %s", setupSession.ExpiresAt.Local().Format("2006-01-02 15:04:05")),
				}, output.BoxOptions{
					MaxWidth: application.StdoutFeatures().Width,
					Unicode:  application.StdoutFeatures().HasUnicode,
				}); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(command.OutOrStdout())
			}

			finalSession, err := waitForSetupSession(command.OutOrStdout(), application.StdoutFeatures(), sdk, setupSession.Id, setupSession.ExpiresAt)
			if err != nil {
				return err
			}

			finalListing, provider, err := resolveSetupListing(sdk, listing, finalSession.ProviderId)
			if err != nil {
				return err
			}

			name, description := setupServerNameAndDescription(finalListing, provider)
			server, err := sdk.MagicMcpServers.Create(&endpoints.MagicMcpServersEndpointCreateBody{
				Name:              stringPtrIfSet(name),
				Description:       stringPtrIfSet(description),
				ConsumerProfileId: &consumer.Profile.Id,
			})
			if err != nil {
				return err
			}

			tips := []string{
				fmt.Sprintf("metorial integrations get %s", server.Id),
				fmt.Sprintf("metorial integrations tools %s", server.Id),
			}
			if finalListing != nil {
				tips = append(tips, fmt.Sprintf("metorial integrations catalog get %s", finalListing.Slug))
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"consumer":      consumer,
					"setup_session": finalSession,
					"provider":      provider,
					"listing":       finalListing,
					"integration":   server,
					"tips":          tips,
				})
			}

			return renderSetupResult(command.OutOrStdout(), application.StdoutFeatures(), finalSession, finalListing, provider, server, tips)
		},
	})

	catalogCommand := &cobra.Command{
		Use:   "catalog",
		Short: "Browse the integration catalog",
	}

	runCatalogList := func(command *cobra.Command, args []string) error {
		runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
		if err != nil {
			return err
		}

		sdk, err := runtime.SDK()
		if err != nil {
			return err
		}

		params := &endpoints.ProviderListingsEndpointListParams{
			Limit: float64Ptr(100),
		}
		if search := strings.TrimSpace(optionalArg(args, 0)); search != "" {
			params.Search = &search
		}

		result, err := sdk.ProviderListings.List(params)
		if err != nil {
			return err
		}

		rows := make([]catalogListRow, 0, len(result.Items))
		for _, item := range result.Items {
			rows = append(rows, catalogListRow{
				Id:          item.Id,
				Slug:        item.Slug,
				Name:        item.Name,
				Description: optionalString(item.Description),
				Publisher:   item.Provider.Publisher.Name,
			})
		}

		tips := []string{}
		if len(result.Items) > 0 {
			tips = append(tips, fmt.Sprintf("metorial integrations catalog get %s", result.Items[0].Slug))
			tips = append(tips, fmt.Sprintf("metorial integrations setup %s", result.Items[0].Slug))
		}

		format, err := output.ParseFormat(rootOptions.format)
		if err != nil {
			return err
		}

		if format != output.FormatStructured {
			return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
				"items":      result.Items,
				"rows":       rows,
				"pagination": result.Pagination,
				"tips":       tips,
			})
		}

		return renderCatalogList(command.OutOrStdout(), application.StdoutFeatures(), rows, tips)
	}

	catalogCommand.AddCommand(&cobra.Command{
		Use:   "list [search]",
		Short: "List provider listings in the integration catalog",
		Args:  cobra.RangeArgs(0, 1),
		RunE:  runCatalogList,
	})
	catalogCommand.AddCommand(&cobra.Command{
		Use:   "search [search]",
		Short: "Search provider listings in the integration catalog",
		Args:  cobra.RangeArgs(0, 1),
		RunE:  runCatalogList,
	})
	catalogCommand.AddCommand(&cobra.Command{
		Use:   "get <provider-listing-slug-or-id>",
		Short: "Show a provider listing and its tools",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			sdk, err := runtime.SDK()
			if err != nil {
				return err
			}

			listing, err := sdk.ProviderListings.Get(args[0])
			if err != nil {
				return err
			}

			tools, err := listProviderTools(sdk, listing.Provider.CurrentVersion)
			if err != nil {
				return err
			}

			tips := []string{
				fmt.Sprintf("metorial integrations setup %s", listing.Slug),
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"listing": listing,
					"tools":   tools,
					"tips":    tips,
				})
			}

			return renderCatalogDetail(command.OutOrStdout(), application.StdoutFeatures(), listing, tools, tips)
		},
	})

	command.AddCommand(catalogCommand)

	command.AddCommand(&cobra.Command{
		Use:   "tools <magic-mcp-server-id>",
		Short: "Show provider details and tools for an integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			sdk, err := runtime.SDK()
			if err != nil {
				return err
			}

			server, err := sdk.MagicMcpServers.Get(args[0])
			if err != nil {
				return err
			}

			templateProviders, err := sdk.SessionTemplatesProviders.List(&endpoints.SessionTemplatesProvidersEndpointListParams{
				Limit:             float64Ptr(100),
				SessionTemplateId: anyPtr(server.SessionTemplateId),
			})
			if err != nil {
				return err
			}

			seen := map[string]bool{}
			providersWithTools := make([]integrationToolsProvider, 0, len(templateProviders.Items))
			for _, item := range templateProviders.Items {
				if seen[item.ProviderId] {
					continue
				}
				seen[item.ProviderId] = true

				provider, err := sdk.Providers.Get(item.ProviderId)
				if err != nil {
					return err
				}

				var listing *providerlistings.ProviderListingsGetOutput
				if strings.TrimSpace(provider.Slug) != "" {
					listing, err = sdk.ProviderListings.Get(provider.Slug)
					if err != nil {
						listing = nil
					}
				}

				tools, err := listProviderTools(sdk, provider.CurrentVersion)
				if err != nil {
					return err
				}

				providersWithTools = append(providersWithTools, integrationToolsProvider{
					Provider: provider,
					Listing:  listing,
					Tools:    tools,
				})
			}

			tips := []string{}
			for _, providerWithTools := range providersWithTools {
				if providerWithTools.Listing != nil {
					tips = append(tips, fmt.Sprintf("metorial integrations catalog get %s", providerWithTools.Listing.Slug))
					break
				}
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"integration": server,
					"providers":   providersWithTools,
					"tips":        tips,
				})
			}

			return renderIntegrationTools(command.OutOrStdout(), application.StdoutFeatures(), server, providersWithTools, tips)
		},
	})

	command.SetHelpTemplate(helpTemplate())
	command.SetUsageTemplate(usageTemplate())
	command.Annotations = map[string]string{
		"metorial:command-category": commandCategoryGeneral,
	}

	return command
}

func getCLIMemberConsumer(runtime config.Runtime) (*consumers.ConsumersGetMemberConsumerOutput, error) {
	response, err := fetch.Execute(runtime, fetch.Options{
		Method: "POST",
		Target: "/get-member-consumer",
		Data:   `{"surface_identifier":"cli"}`,
	}, strings.NewReader(""))
	if err != nil {
		return nil, err
	}

	var consumer consumers.ConsumersGetMemberConsumerOutput
	if err := json.Unmarshal(response.Body, &consumer); err != nil {
		return nil, fmt.Errorf("metorial: failed to decode member consumer: %w", err)
	}

	return &consumer, nil
}

func waitForSetupSession(writer io.Writer, features terminal.Features, sdk *metorial.MetorialSdk, setupSessionID string, expiresAt time.Time) (*setupsessions.ProviderDeploymentsSetupSessionsGetOutput, error) {
	spinner := terminal.NewSpinner(writer, features, "Waiting for setup to complete")
	spinner.Start()
	defer spinner.Stop()

	for {
		setupSession, err := sdk.ProviderDeploymentsSetupSessions.Get(setupSessionID)
		if err != nil {
			return nil, err
		}

		switch setupSession.Status {
		case "completed":
			return setupSession, nil
		case "failed":
			return nil, fmt.Errorf("metorial: setup session %s failed", setupSession.Id)
		case "expired":
			return nil, fmt.Errorf("metorial: setup session %s expired", setupSession.Id)
		case "archived":
			return nil, fmt.Errorf("metorial: setup session %s was archived before completion", setupSession.Id)
		}

		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("metorial: setup session %s expired before completion", setupSession.Id)
		}

		time.Sleep(2 * time.Second)
	}
}

func resolveSetupListing(sdk *metorial.MetorialSdk, listing *providerlistings.ProviderListingsGetOutput, providerID *string) (*providerlistings.ProviderListingsGetOutput, *providers.ProvidersGetOutput, error) {
	if listing != nil {
		provider, err := sdk.Providers.Get(listing.Provider.Id)
		if err != nil {
			return listing, nil, err
		}
		return listing, provider, nil
	}

	if providerID == nil || strings.TrimSpace(*providerID) == "" {
		return nil, nil, fmt.Errorf("metorial: setup session completed without a provider_id")
	}

	provider, err := sdk.Providers.Get(*providerID)
	if err != nil {
		return nil, nil, err
	}

	if strings.TrimSpace(provider.Slug) == "" {
		return nil, provider, nil
	}

	listing, err = sdk.ProviderListings.Get(provider.Slug)
	if err != nil {
		return nil, provider, nil
	}

	return listing, provider, nil
}

func listProviderTools(sdk *metorial.MetorialSdk, version any) ([]providertools.ProvidersToolsListOutputItems, error) {
	switch typed := version.(type) {
	case *providerlistings.ProviderListingsGetOutputProviderCurrentVersion:
		if typed == nil || strings.TrimSpace(typed.Id) == "" {
			return nil, nil
		}
		result, err := sdk.ProvidersTools.List(&endpoints.ProvidersToolsEndpointListParams{
			Limit:             float64Ptr(100),
			ProviderVersionId: typed.Id,
		})
		if err != nil {
			return nil, err
		}
		return result.Items, nil
	case *providers.ProvidersGetOutputCurrentVersion:
		if typed == nil || strings.TrimSpace(typed.Id) == "" {
			return nil, nil
		}
		result, err := sdk.ProvidersTools.List(&endpoints.ProvidersToolsEndpointListParams{
			Limit:             float64Ptr(100),
			ProviderVersionId: typed.Id,
		})
		if err != nil {
			return nil, err
		}
		return result.Items, nil
	default:
		return nil, nil
	}
}

func renderIntegrationsList(writer io.Writer, features terminal.Features, consumer *consumers.ConsumersGetMemberConsumerOutput, rows []integrationListRow, tips []string) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Bold("Your Integrations"))
	_, _ = fmt.Fprintf(writer, "%s\n\n", colors.Muted(fmt.Sprintf("Consumer %s (%s)", consumer.Name, consumer.Id)))

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
		name := row.Id
		if strings.TrimSpace(row.Name) != "" {
			name = colors.Bold(row.Name) + "\n" + colors.Muted(row.Id)
		}

		table.Rows = append(table.Rows, []string{
			name,
			row.Status,
			row.EndpointUrl,
			row.Description,
		})
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(writer, "(no integrations)")
	} else if err := table.Render(writer); err != nil {
		return err
	}

	return renderTips(writer, features, tips)
}

func renderIntegrationDetail(writer io.Writer, features terminal.Features, server *magicmcpservers.MagicMcpServersGetOutput, tips []string) error {
	colors := terminal.NewColorizer(features)
	title := server.Id
	if strings.TrimSpace(optionalString(server.Name)) != "" {
		title = optionalString(server.Name)
	}

	_, _ = fmt.Fprintln(writer, colors.Bold(title))
	_, _ = fmt.Fprintf(writer, "%s\n\n", colors.Muted(server.Id))

	items := []output.DataListItem{
		{Label: "Status", Value: server.Status},
		{Label: "Session Template", Value: server.SessionTemplateId},
	}
	if description := optionalString(server.Description); description != "" {
		items = append(items, output.DataListItem{Label: "Description", Value: description})
	}

	if err := output.RenderDataList(writer, items); err != nil {
		return err
	}

	if len(server.Endpoints) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Endpoints"))

		table := output.Table{
			Columns: []string{
				colors.Accent("Alias"),
				colors.Accent("URL"),
			},
			Features: features,
			MaxWidth: features.Width,
		}
		for _, endpoint := range server.Endpoints {
			table.Rows = append(table.Rows, []string{endpoint.Alias, endpoint.Url})
		}
		if err := table.Render(writer); err != nil {
			return err
		}
	}

	return renderTips(writer, features, tips)
}

func renderCatalogList(writer io.Writer, features terminal.Features, rows []catalogListRow, tips []string) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Bold("Integration Catalog"))
	_, _ = fmt.Fprintf(writer, "%s\n\n", colors.Muted("Browse installable provider listings."))

	table := output.Table{
		Columns: []string{
			colors.Accent("Slug"),
			colors.Accent("Name"),
			colors.Accent("Publisher"),
			colors.Accent("Description"),
		},
		Features: features,
		MaxWidth: features.Width,
	}

	for _, row := range rows {
		table.Rows = append(table.Rows, []string{
			colors.Bold(row.Slug) + "\n" + colors.Muted(row.Id),
			row.Name,
			row.Publisher,
			row.Description,
		})
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(writer, "(no listings)")
	} else if err := table.Render(writer); err != nil {
		return err
	}

	return renderTips(writer, features, tips)
}

func renderCatalogDetail(writer io.Writer, features terminal.Features, listing *providerlistings.ProviderListingsGetOutput, tools []providertools.ProvidersToolsListOutputItems, tips []string) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Bold(listing.Name))
	_, _ = fmt.Fprintf(writer, "%s\n\n", colors.Muted(listing.Slug))

	items := []output.DataListItem{
		{Label: "Listing", Value: listing.Id},
		{Label: "Provider", Value: fmt.Sprintf("%s (%s)", listing.Provider.Name, listing.Provider.Id)},
		{Label: "Publisher", Value: listing.Provider.Publisher.Name},
	}
	if listing.Provider.CurrentVersion != nil {
		items = append(items, output.DataListItem{
			Label: "Current Version",
			Value: fmt.Sprintf("%s (%s)", listing.Provider.CurrentVersion.Version, listing.Provider.CurrentVersion.Id),
		})
	}
	if description := optionalString(listing.Description); description != "" {
		items = append(items, output.DataListItem{Label: "Description", Value: description})
	}

	if err := output.RenderDataList(writer, items); err != nil {
		return err
	}

	if strings.TrimSpace(optionalString(listing.Readme)) != "" {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Readme"))
		_, _ = fmt.Fprintln(writer, optionalString(listing.Readme))
	}

	if err := renderToolsTable(writer, features, tools); err != nil {
		return err
	}

	return renderTips(writer, features, tips)
}

func renderIntegrationTools(writer io.Writer, features terminal.Features, server *magicmcpservers.MagicMcpServersGetOutput, providersWithTools []integrationToolsProvider, tips []string) error {
	colors := terminal.NewColorizer(features)
	title := server.Id
	if name := optionalString(server.Name); name != "" {
		title = name
	}

	_, _ = fmt.Fprintln(writer, colors.Bold(title))
	_, _ = fmt.Fprintf(writer, "%s\n", colors.Muted(server.Id))
	_, _ = fmt.Fprintf(writer, "%s\n\n", colors.Muted(fmt.Sprintf("Session template %s", server.SessionTemplateId)))

	if len(providersWithTools) == 0 {
		_, _ = fmt.Fprintln(writer, "(no linked providers)")
		return renderTips(writer, features, tips)
	}

	for index, providerWithTools := range providersWithTools {
		if index > 0 {
			_, _ = fmt.Fprintln(writer)
		}

		title := providerWithTools.Provider.Name
		if providerWithTools.Listing != nil {
			title = providerWithTools.Listing.Name
		}

		_, _ = fmt.Fprintln(writer, colors.Accent(title))
		if providerWithTools.Listing != nil {
			_, _ = fmt.Fprintf(writer, "%s\n", colors.Muted(providerWithTools.Listing.Slug))
		} else {
			_, _ = fmt.Fprintf(writer, "%s\n", colors.Muted(providerWithTools.Provider.Slug))
		}

		description := optionalString(providerWithTools.Provider.Description)
		if providerWithTools.Listing != nil && optionalString(providerWithTools.Listing.Description) != "" {
			description = optionalString(providerWithTools.Listing.Description)
		}
		if description != "" {
			_, _ = fmt.Fprintf(writer, "%s\n", description)
		}

		if providerWithTools.Listing != nil && strings.TrimSpace(optionalString(providerWithTools.Listing.Readme)) != "" {
			_, _ = fmt.Fprintln(writer)
			_, _ = fmt.Fprintln(writer, colors.Muted("Readme"))
			_, _ = fmt.Fprintln(writer, optionalString(providerWithTools.Listing.Readme))
		}

		if err := renderToolsTable(writer, features, providerWithTools.Tools); err != nil {
			return err
		}
	}

	return renderTips(writer, features, tips)
}

func renderSetupResult(writer io.Writer, features terminal.Features, setupSession *setupsessions.ProviderDeploymentsSetupSessionsGetOutput, listing *providerlistings.ProviderListingsGetOutput, provider *providers.ProvidersGetOutput, server *magicmcpservers.MagicMcpServersCreateOutput, tips []string) error {
	colors := terminal.NewColorizer(features)
	title := server.Id
	if name := optionalString(server.Name); name != "" {
		title = name
	}

	_, _ = fmt.Fprintln(writer, colors.Success("Integration Ready"))
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Bold(title))
	_, _ = fmt.Fprintf(writer, "%s\n\n", colors.Muted(server.Id))

	items := []output.DataListItem{
		{Label: "Status", Value: server.Status},
		{Label: "Setup Session", Value: setupSession.Id},
	}

	if listing != nil {
		items = append(items, output.DataListItem{
			Label: "Catalog Listing",
			Value: fmt.Sprintf("%s (%s)", listing.Slug, listing.Id),
		})
	} else if provider != nil {
		items = append(items, output.DataListItem{
			Label: "Provider",
			Value: fmt.Sprintf("%s (%s)", provider.Name, provider.Id),
		})
	}

	if err := output.RenderDataList(writer, items); err != nil {
		return err
	}

	if len(server.Endpoints) > 0 {
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Accent("Endpoints"))
		table := output.Table{
			Columns: []string{
				colors.Accent("Alias"),
				colors.Accent("URL"),
			},
			Features: features,
			MaxWidth: features.Width,
		}
		for _, endpoint := range server.Endpoints {
			table.Rows = append(table.Rows, []string{endpoint.Alias, endpoint.Url})
		}
		if err := table.Render(writer); err != nil {
			return err
		}
	}

	return renderTips(writer, features, tips)
}

func renderToolsTable(writer io.Writer, features terminal.Features, tools []providertools.ProvidersToolsListOutputItems) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Accent("Tools"))

	if len(tools) == 0 {
		_, _ = fmt.Fprintln(writer, "(no tools)")
		return nil
	}

	table := output.Table{
		Columns: []string{
			colors.Accent("Key"),
			colors.Accent("Name"),
			colors.Accent("Description"),
		},
		Features: features,
		MaxWidth: features.Width,
	}

	for _, tool := range tools {
		table.Rows = append(table.Rows, []string{
			colors.Bold(tool.Key) + "\n" + colors.Muted(tool.Id),
			tool.Name,
			optionalString(tool.Description),
		})
	}

	return table.Render(writer)
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

func primaryServerURL(endpoints []magicmcpservers.MagicMcpServersListOutputItemsEndpoints) string {
	if len(endpoints) == 0 {
		return ""
	}
	return endpoints[0].Url
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPtrIfSet(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	value = strings.TrimSpace(value)
	return &value
}

func anyPtr(value any) *any {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func setupServerNameAndDescription(listing *providerlistings.ProviderListingsGetOutput, provider *providers.ProvidersGetOutput) (string, string) {
	if listing != nil {
		return listing.Name, optionalString(listing.Description)
	}
	if provider != nil {
		return provider.Name, optionalString(provider.Description)
	}
	return "", ""
}
