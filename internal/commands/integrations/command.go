package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	metorial "github.com/metorial/metorial-go/v1"
	"github.com/metorial/metorial-go/v1/endpoints"
	"github.com/metorial/metorial-go/v1/resources/consumers"
	"github.com/metorial/metorial-go/v1/resources/magicmcpservers"
	magicmcpserverprovider "github.com/metorial/metorial-go/v1/resources/magicmcpservers/provider"
	setupsessions "github.com/metorial/metorial-go/v1/resources/providerdeployments/setupsessions"
	providerlistings "github.com/metorial/metorial-go/v1/resources/providerlistings"
	"github.com/metorial/metorial-go/v1/resources/providers"
	providertools "github.com/metorial/metorial-go/v1/resources/providers/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type integrationToolsProvider struct {
	Provider *providers.ProvidersGetOutput               `json:"provider,omitempty"`
	Listing  *providerlistings.ProviderListingsGetOutput `json:"listing,omitempty"`
}

type integrationListRow struct {
	Id          string `json:"id"`
	Alias       string `json:"alias,omitempty"`
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

type integrationCallOptions struct {
	Data string
}

type paginationOptions struct {
	Limit  float64
	After  string
	Before string
	Cursor string
}

type rootOptionsView struct {
	apiKey   string
	apiHost  string
	instance string
	profile  string
	format   string
}

func newRootOptionsView(options *commandutil.RootOptions) *rootOptionsView {
	if options == nil {
		return &rootOptionsView{}
	}

	return &rootOptionsView{
		apiKey:   options.APIKey,
		apiHost:  options.APIHost,
		instance: options.Instance,
		profile:  options.Profile,
		format:   options.Format,
	}
}

func writeValue(writer io.Writer, features terminal.Features, formatInput string, value any) error {
	return commandutil.WriteValue(writer, features, formatInput, value)
}

func helpTemplate() string {
	return commandutil.HelpTemplate()
}

func usageTemplate() string {
	return commandutil.UsageTemplate()
}

const commandCategoryIntegrations = commandutil.CommandCategoryIntegrations

func optionalArg(args []string, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	return args[index]
}

func firstNonEmpty(values ...string) string {
	return commandutil.FirstNonEmpty(values...)
}

func NewCommand(ctx commandutil.Context) *cobra.Command {
	return newIntegrationsCommand(ctx.App, newRootOptionsView(ctx.Options))
}

func newIntegrationsCommand(application *app.App, rootOptions *rootOptionsView) *cobra.Command {
	browserShell := commandutil.BrowserShellEnabled()

	command := &cobra.Command{
		Use:     "integrations",
		Aliases: []string{"integration"},
		Short:   "Browse and set up your integrations",
	}

	integrationListPagination := paginationOptions{Limit: 15}
	catalogPagination := paginationOptions{Limit: 15}

	listCommand := &cobra.Command{
		Use:    "list [search]",
		Hidden: true,
		Short:  "List your integrations, optionally filtering by a search term",
		Args:   cobra.RangeArgs(0, 1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			consumer, err := getCLIMemberConsumer(runtime)
			if err != nil {
				return err
			}
			consumerSDK, err := newConsumerProfileSDK(runtime, consumer.Profile.Id)
			if err != nil {
				return err
			}

			params := &endpoints.MagicMcpServersEndpointListParams{}
			applyPaginationOptionsToMagicMcpServers(params, integrationListPagination)
			if search := strings.TrimSpace(optionalArg(args, 0)); search != "" {
				params.Search = &search
			}

			result, err := consumerSDK.MagicMcpServers.List(params)
			if err != nil {
				return err
			}

			rows := make([]integrationListRow, 0, len(result.Items))
			for _, item := range result.Items {
				rows = append(rows, integrationListRow{
					Id:          item.Id,
					Alias:       primaryServerAlias(item.Endpoints),
					Status:      item.Status,
					Name:        optionalString(item.Name),
					Description: optionalString(item.Description),
					EndpointUrl: primaryServerURL(item.Endpoints),
				})
			}

			tips := []string{}
			if len(result.Items) > 0 {
				identifier := integrationListIdentifier(result.Items[0])
				tips = append(tips, fmt.Sprintf("metorial integrations get %s", identifier))
				tips = append(tips, fmt.Sprintf("metorial integrations tools %s", identifier))
				if !browserShell {
					tips = append(tips, fmt.Sprintf("metorial integrations install codex %s", identifier))
				}
			}
			tips = append(tips, paginationTipsForIntegrationList(args, integrationListPagination, result.Items, result.Pagination)...)
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
	}
	addPaginationFlags(listCommand, &integrationListPagination, "integrations")
	command.AddCommand(listCommand)

	command.AddCommand(&cobra.Command{
		Use:   "get <integration-id>",
		Short: "View an integration's details and tools",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			consumer, err := getCLIMemberConsumer(runtime)
			if err != nil {
				return err
			}
			consumerSDK, err := newConsumerProfileSDK(runtime, consumer.Profile.Id)
			if err != nil {
				return err
			}

			server, err := consumerSDK.MagicMcpServers.Get(args[0])
			if err != nil {
				return err
			}

			tips := []string{
				fmt.Sprintf("metorial integrations tools %s", integrationGetIdentifier(server)),
			}
			if !browserShell {
				tips = append(tips,
					fmt.Sprintf("metorial integrations install codex %s", integrationGetIdentifier(server)),
					fmt.Sprintf("metorial integrations install custom %s", integrationGetIdentifier(server)),
				)
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
		Short: "Configure a new integration from the Metorial catalog",
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

			name := "Metorial CLI"
			sessionType := "auto"
			createBody := &endpoints.ProviderDeploymentsSetupSessionsEndpointCreateBody{
				ConsumerId: &consumer.Id,
				Name:       &name,
				Type:       &sessionType,
				Configuration: &map[string]any{
					"ui": &map[string]any{
						"layout": "light",
					},
					"tool_filters": &map[string]any{
						"enabled": true,
					},
				},
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

			if format == output.FormatStructured && setupSessionRequiresUserAction(setupSession.Status, setupSession.Url) {
				colors := terminal.NewColorizer(application.StdoutFeatures())
				browserOpenSupported := browser.Supported()
				openSummary := fmt.Sprintf("%s\n%s", colors.Notice("Open this URL to continue setup"), colors.Bold(setupSession.Url))
				if browserOpenSupported {
					openSummary = fmt.Sprintf("%s\n%s", colors.Success("Opened setup in your browser"), colors.Bold(setupSession.Url))
				}

				if err := output.RenderBox(command.OutOrStdout(), []string{
					openSummary,
					fmt.Sprintf("%s %s", colors.Muted("Setup session:"), colors.Bold(setupSession.Id)),
					fmt.Sprintf("%s %s", colors.Muted("Expires:"), setupSession.ExpiresAt.Local().Format("2006-01-02 15:04:05")),
				}, output.BoxOptions{
					MaxWidth: application.StdoutFeatures().Width,
					Unicode:  application.StdoutFeatures().HasUnicode,
				}); err != nil {
					return err
				}
				if browserOpenSupported {
					_ = browser.Open(setupSession.Url)
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

			sessionTemplateProvider, err := attachSetupSessionProviderToMagicServer(runtime, sdk, finalSession, server)
			if err != nil {
				return err
			}

			tips := []string{
				fmt.Sprintf("metorial integrations get %s", integrationCreateIdentifier(server)),
				fmt.Sprintf("metorial integrations tools %s", integrationCreateIdentifier(server)),
			}
			if !browserShell {
				tips = append(tips,
					fmt.Sprintf("metorial integrations install codex %s", integrationCreateIdentifier(server)),
					fmt.Sprintf("metorial integrations install custom %s", integrationCreateIdentifier(server)),
				)
			}
			if finalListing != nil {
				tips = append(tips, fmt.Sprintf("metorial integrations catalog get %s", finalListing.Slug))
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"consumer":          consumer,
					"setup_session":     finalSession,
					"provider":          provider,
					"listing":           finalListing,
					"integration":       server,
					"template_provider": sessionTemplateProvider,
					"tips":              tips,
				})
			}

			return renderSetupResult(command.OutOrStdout(), application.StdoutFeatures(), finalSession, finalListing, provider, server, sessionTemplateProvider, tips)
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

		params := &endpoints.ProviderListingsEndpointListParams{}
		applyPaginationOptionsToProviderListings(params, catalogPagination)
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
				Id:          item.Provider.Id,
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
		tips = append(tips, paginationTipsForCatalogList(command, args, catalogPagination, result.Items, result.Pagination)...)

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
	addPaginationFlags(catalogCommand.Commands()[0], &catalogPagination, "provider listings")

	catalogSearchCommand := &cobra.Command{
		Use:   "search [search]",
		Short: "Search provider listings in the integration catalog",
		Args:  cobra.RangeArgs(0, 1),
		RunE:  runCatalogList,
	}
	addPaginationFlags(catalogSearchCommand, &catalogPagination, "provider listings")
	catalogCommand.AddCommand(catalogSearchCommand)
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
	searchCommand := &cobra.Command{
		Use:   "search [search]",
		Short: "Search provider listings in the integration catalog",
		Args:  cobra.RangeArgs(0, 1),
		RunE:  runCatalogList,
	}
	addPaginationFlags(searchCommand, &catalogPagination, "provider listings")
	command.AddCommand(searchCommand)

	if !browserShell {
		clientCommand := &cobra.Command{
			Use:   "client",
			Short: "Inspect supported local MCP clients",
		}
		clientCommand.AddCommand(&cobra.Command{
			Use:   "list",
			Short: "List supported MCP clients and installation availability",
			RunE: func(command *cobra.Command, args []string) error {
				rows := integrationClientRows()
				tips := []string{
					"metorial integrations install codex <integration-id>",
					"metorial integrations install custom <integration-id>",
				}

				format, err := output.ParseFormat(rootOptions.format)
				if err != nil {
					return err
				}
				if format != output.FormatStructured {
					return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
						"items": rows,
						"tips":  tips,
					})
				}

				return renderIntegrationClientList(command.OutOrStdout(), application.StdoutFeatures(), rows, tips)
			},
		})
		command.AddCommand(clientCommand)
	}

	installCommand := &cobra.Command{
		Use:   "install <client-identifier> <integration-id>",
		Short: "Install an integration into a local MCP client",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			consumer, err := getCLIMemberConsumer(runtime)
			if err != nil {
				return err
			}
			consumerSDK, err := newConsumerProfileSDK(runtime, consumer.Profile.Id)
			if err != nil {
				return err
			}

			server, err := consumerSDK.MagicMcpServers.Get(args[1])
			if err != nil {
				return err
			}

			adapter, err := integrationClientByID(args[0])
			if err != nil {
				return err
			}
			detection := adapter.detect()
			if !detection.Usable {
				return fmt.Errorf("metorial: %s is not installed or cannot be used on this system. Run `metorial integrations client list` for supported clients", adapter.label())
			}

			token, err := createMagicMcpInstallToken(consumerSDK, server.Id)
			if err != nil {
				return err
			}
			endpointURL, err := buildMagicMcpInstallURL(server, token.Secret)
			if err != nil {
				return err
			}

			transport := selectInstallTransport(adapter.capabilities())
			plan, err := adapter.buildInstallPlan(remoteServerDefinition{
				Name:      integrationGetIdentifier(server),
				URL:       endpointURL,
				Transport: transport,
			})
			if err != nil {
				return err
			}

			if err := executeIntegrationInstallPlan(plan); err != nil {
				return err
			}

			result := integrationInstallResult{
				Client:      adapter.label(),
				Method:      string(plan.method()),
				Transport:   string(plan.transport()),
				Integration: integrationGetIdentifier(server),
				EndpointURL: endpointURL,
				TokenID:     token.Id,
			}
			switch typed := plan.(type) {
			case commandInstallPlan:
				result.Command = &typed
			case fileInstallPlan:
				result.File = &typed
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}
			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, result)
			}
			return renderIntegrationInstallResult(command.OutOrStdout(), application.StdoutFeatures(), result)
		},
	}
	installCommand.AddCommand(&cobra.Command{
		Use:   "custom <integration-id>",
		Short: "Create a custom installation token and show the endpoint details",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			consumer, err := getCLIMemberConsumer(runtime)
			if err != nil {
				return err
			}
			consumerSDK, err := newConsumerProfileSDK(runtime, consumer.Profile.Id)
			if err != nil {
				return err
			}

			server, err := consumerSDK.MagicMcpServers.Get(args[0])
			if err != nil {
				return err
			}
			token, err := createMagicMcpInstallToken(consumerSDK, server.Id)
			if err != nil {
				return err
			}
			endpointURL, err := buildMagicMcpInstallURL(server, token.Secret)
			if err != nil {
				return err
			}

			result := integrationInstallResult{
				Client:      "custom",
				Method:      "custom",
				Transport:   string(installTransportStreamableHTTP),
				Integration: integrationGetIdentifier(server),
				EndpointURL: endpointURL,
				TokenID:     token.Id,
				Token:       token.Secret,
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}
			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, result)
			}
			return renderCustomInstallResult(command.OutOrStdout(), application.StdoutFeatures(), result)
		},
	})
	if !browserShell {
		command.AddCommand(installCommand)
	}

	command.AddCommand(&cobra.Command{
		Use:   "tools <integration-id>",
		Short: "View provider details and tools for an integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			ctx := command.Context()
			if ctx == nil {
				ctx = context.Background()
			}

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
			consumerSDK, err := newConsumerProfileSDK(runtime, consumer.Profile.Id)
			if err != nil {
				return err
			}

			server, err := consumerSDK.MagicMcpServers.Get(args[0])
			if err != nil {
				return err
			}

			tokenSecret, err := ensureMagicMCPTokenSecret(consumerSDK)
			if err != nil {
				return err
			}

			mcpClient, err := connectMagicMCP(ctx, tokenSecret, consumer.Profile.Id, server)
			if err != nil {
				return err
			}
			defer mcpClient.Close()

			liveTools, err := mcpClient.ListTools(ctx)
			if err != nil {
				return err
			}

			templateProviders, err := listMagicMcpServerProviders(runtime, consumerSDK, server.Id)
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

				providersWithTools = append(providersWithTools, integrationToolsProvider{
					Provider: provider,
					Listing:  listing,
				})
			}

			tips := []string{}
			for _, providerWithTools := range providersWithTools {
				if providerWithTools.Listing != nil {
					tips = append(tips, fmt.Sprintf("metorial integrations catalog get %s", providerWithTools.Listing.Slug))
					break
				}
			}
			if len(liveTools) > 0 {
				identifier := integrationGetIdentifier(server)
				toolKey := liveTools[0].Name
				tips = append(tips, fmt.Sprintf("metorial integrations install codex %s", identifier))
				tips = append(tips, fmt.Sprintf("metorial integrations install custom %s", identifier))
				tips = append(tips, fmt.Sprintf("metorial integrations schema %s %s", identifier, toolKey))
				tips = append(tips, fmt.Sprintf("metorial integrations call %s %s --data '{}'", identifier, toolKey))
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return writeValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, map[string]any{
					"integration": server,
					"providers":   providersWithTools,
					"tools":       liveTools,
					"tips":        tips,
				})
			}

			return renderIntegrationTools(command.OutOrStdout(), application.StdoutFeatures(), server, providersWithTools, liveTools, tips)
		},
	})

	command.AddCommand(&cobra.Command{
		Use:   "schema <integration-id> <tool-key>",
		Short: "View the input schema for an integration tool",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			ctx := command.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			_, _, mcpClient, err := resolveIntegrationMCPClient(ctx, application, rootOptions, args[0])
			if err != nil {
				return err
			}
			defer mcpClient.Close()

			tool, err := mcpClient.FindToolByName(ctx, args[1])
			if err != nil {
				return err
			}

			schemaValue := tool.InputSchema
			if schemaValue == nil {
				schemaValue = map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": true,
				}
			}

			return writeIntegrationMachineValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, schemaValue)
		},
	})

	callOptions := &integrationCallOptions{}
	callCommand := &cobra.Command{
		Use:   "call <integration-id> <tool-key>",
		Short: "Call an integration tool over MCP",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			ctx := command.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			server, _, mcpClient, err := resolveIntegrationMCPClient(ctx, application, rootOptions, args[0])
			if err != nil {
				return err
			}
			defer mcpClient.Close()

			tool, err := mcpClient.FindToolByName(ctx, args[1])
			if err != nil {
				return err
			}

			input, preview, err := resolveIntegrationToolInput(application, callOptions.Data)
			if err != nil {
				return err
			}

			if err := validateIntegrationToolInput(tool, input, preview, application.StderrFeatures()); err != nil {
				return err
			}

			result, err := mcpClient.CallTool(ctx, tool.Name, input)
			if err != nil {
				return err
			}
			if result.IsError {
				return formatMCPToolResultError(application.StderrFeatures(), server, tool, result)
			}

			return writeIntegrationMachineValue(command.OutOrStdout(), application.StdoutFeatures(), rootOptions.format, normalizeMCPCallResult(result))
		},
	}
	callCommand.Flags().StringVarP(&callOptions.Data, "data", "d", "", "JSON tool input, or @- to read from stdin")
	command.AddCommand(callCommand)

	command.SetHelpTemplate(helpTemplate())
	command.SetUsageTemplate(usageTemplate())
	command.Annotations = map[string]string{
		"metorial:command-category": commandCategoryIntegrations,
		"metorial:help-summary": strings.Join([]string{
			"search [search]               Shortcut for catalog search",
			"setup [listing]               Create and finish an integration setup session",
			"install <client> <id>         Install an integration into a local client",
			"client list                   List supported local MCP clients",
			"schema <integration> <tool>   Show the MCP input schema for a tool",
			"call <integration> <tool>     Validate input and call a tool over MCP",
			"catalog list [search]         Browse installable provider listings",
			"catalog get <listing>         Show listing details, readme, and tools",
			"tools <integration-id>   Show providers and tools for an integration",
		}, "\n"),
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

func newConsumerProfileSDK(runtime config.Runtime, consumerProfileID string) (*metorial.MetorialSdk, error) {
	if err := runtime.RequireAPIKey(); err != nil {
		return nil, err
	}

	headers := map[string]string{
		"metorial-consumer-profile-id": strings.TrimSpace(consumerProfileID),
	}
	if strings.TrimSpace(runtime.InstanceID) != "" {
		headers["metorial-instance-id"] = strings.TrimSpace(runtime.InstanceID)
	}

	return metorial.New(
		metorial.WithAPIKey(runtime.APIKey),
		metorial.WithAPIHost(runtime.APIHost),
		metorial.WithHeaders(headers),
	)
}

func ensureMagicMCPTokenSecret(sdk *metorial.MetorialSdk) (string, error) {
	if sdk == nil {
		return "", fmt.Errorf("metorial: consumer-scoped SDK is required")
	}

	status := any("active")
	tokens, err := sdk.MagicMcpTokens.List(&endpoints.MagicMcpTokensEndpointListParams{
		Limit:  float64Ptr(15),
		Status: &status,
	})
	if err != nil {
		return "", err
	}

	for _, token := range tokens.Items {
		if strings.TrimSpace(token.Status) == "active" && strings.TrimSpace(token.Secret) != "" {
			return strings.TrimSpace(token.Secret), nil
		}
	}

	name := "Metorial CLI"
	token, err := sdk.MagicMcpTokens.Create(&endpoints.MagicMcpTokensEndpointCreateBody{
		Name: name,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token.Secret) == "" {
		return "", fmt.Errorf("metorial: created magic MCP token %s without a secret", token.Id)
	}

	return strings.TrimSpace(token.Secret), nil
}

func listMagicMcpServerProviders(runtime config.Runtime, sdk *metorial.MetorialSdk, magicMcpServerID string) (*magicmcpserverprovider.MagicMcpServersProviderListOutput, error) {
	response, err := consumerSDKFetch(runtime, sdk, "GET", fmt.Sprintf("/magic-mcp-servers/%s/provider?limit=15", strings.TrimSpace(magicMcpServerID)), nil)
	if err != nil {
		return nil, err
	}

	var providers magicmcpserverprovider.MagicMcpServersProviderListOutput
	if err := json.Unmarshal(response.Body, &providers); err != nil {
		return nil, fmt.Errorf("metorial: failed to decode magic MCP providers for %s: %w", magicMcpServerID, err)
	}

	return &providers, nil
}

func createMagicMcpServerProvider(runtime config.Runtime, sdk *metorial.MetorialSdk, magicMcpServerID string, body map[string]any) (*magicmcpserverprovider.MagicMcpServersProviderCreateOutput, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to encode magic MCP provider assignment: %w", err)
	}

	response, err := consumerSDKFetch(runtime, sdk, "POST", fmt.Sprintf("/magic-mcp-servers/%s/provider", strings.TrimSpace(magicMcpServerID)), payload)
	if err != nil {
		return nil, err
	}

	var providerAssignment magicmcpserverprovider.MagicMcpServersProviderCreateOutput
	if err := json.Unmarshal(response.Body, &providerAssignment); err != nil {
		return nil, fmt.Errorf("metorial: failed to decode magic MCP provider assignment for %s: %w", magicMcpServerID, err)
	}

	return &providerAssignment, nil
}

func resolveIntegrationMCPClient(ctx context.Context, application *app.App, rootOptions *rootOptionsView, integrationID string) (*magicmcpservers.MagicMcpServersGetOutput, *consumers.ConsumersGetMemberConsumerOutput, *magicMCPClient, error) {
	runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
	if err != nil {
		return nil, nil, nil, err
	}

	consumer, err := getCLIMemberConsumer(runtime)
	if err != nil {
		return nil, nil, nil, err
	}

	consumerSDK, err := newConsumerProfileSDK(runtime, consumer.Profile.Id)
	if err != nil {
		return nil, nil, nil, err
	}

	server, err := consumerSDK.MagicMcpServers.Get(integrationID)
	if err != nil {
		return nil, nil, nil, err
	}

	tokenSecret, err := ensureMagicMCPTokenSecret(consumerSDK)
	if err != nil {
		return nil, nil, nil, err
	}

	mcpClient, err := connectMagicMCP(ctx, tokenSecret, consumer.Profile.Id, server)
	if err != nil {
		return nil, nil, nil, err
	}

	return server, consumer, mcpClient, nil
}

func consumerSDKFetch(runtime config.Runtime, sdk *metorial.MetorialSdk, method string, target string, body []byte) (*metorial.RawResponse, error) {
	if sdk == nil {
		return nil, fmt.Errorf("metorial: consumer-scoped SDK is required")
	}

	requestURL, err := fetch.ResolveURL(runtime.APIHostURL, target)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{}
	if len(body) > 0 {
		headers["Content-Type"] = "application/json"
	}

	return sdk.Fetch(&metorial.RawRequest{
		Method:  method,
		URL:     requestURL.String(),
		Headers: headers,
		Body:    body,
	})
}

func resolveIntegrationToolInput(application *app.App, data string) (map[string]any, string, error) {
	raw, err := readIntegrationToolInput(application, data)
	if err != nil {
		return nil, "", err
	}

	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, "{}", nil
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, "", fmt.Errorf("%s", formatIntegrationInputParseError(application.StderrFeatures(), raw, err))
	}

	object, ok := value.(map[string]any)
	if !ok {
		preview := previewJSON(value)
		return nil, preview, fmt.Errorf("%s", formatIntegrationInputObjectError(application.StderrFeatures(), preview))
	}

	return object, previewJSON(object), nil
}

func readIntegrationToolInput(application *app.App, data string) (string, error) {
	switch strings.TrimSpace(data) {
	case "", "@-", "-":
	default:
		return data, nil
	}

	if strings.TrimSpace(data) == "@-" || strings.TrimSpace(data) == "-" {
		payload, err := io.ReadAll(application.Stdin)
		if err != nil {
			return "", fmt.Errorf("metorial: failed to read tool input from stdin: %w", err)
		}
		return string(payload), nil
	}

	if file, ok := application.Stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return "{}", nil
	}

	payload, err := io.ReadAll(application.Stdin)
	if err != nil {
		return "", fmt.Errorf("metorial: failed to read tool input from stdin: %w", err)
	}
	if strings.TrimSpace(string(payload)) == "" {
		return "{}", nil
	}

	return string(payload), nil
}

func validateIntegrationToolInput(tool *mcp.Tool, input map[string]any, preview string, features terminal.Features) error {
	schemaValue := map[string]any{
		"type": "object",
	}
	if tool != nil && tool.InputSchema != nil {
		typed, err := normalizeSchemaObject(tool.InputSchema)
		if err != nil {
			return fmt.Errorf("metorial: failed to parse MCP input schema for tool %q: %w", tool.Name, err)
		}
		schemaValue = typed
	}

	schemaBytes, err := json.Marshal(schemaValue)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode input schema for tool %q: %w", tool.Name, err)
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("metorial: failed to decode input schema for tool %q: %w", tool.Name, err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("metorial: failed to resolve input schema for tool %q: %w", tool.Name, err)
	}

	if err := resolved.Validate(input); err != nil {
		return fmt.Errorf("%s", formatIntegrationInputValidationError(features, tool, preview, err))
	}

	return nil
}

func normalizeSchemaObject(value any) (map[string]any, error) {
	if typed, ok := value.(map[string]any); ok {
		return typed, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}

	return decoded, nil
}

func previewJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}

	text := string(encoded)
	if len(text) > 600 {
		return text[:600] + "\n..."
	}
	return text
}

func formatIntegrationInputValidationError(features terminal.Features, tool *mcp.Tool, preview string, validationErr error) string {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Input validation failed"),
		fmt.Sprintf("Tool: %s", colors.Bold(firstNonEmpty(tool.Title, tool.Name))),
		fmt.Sprintf("Key: %s", tool.Name),
		"",
		colors.Accent("Why"),
		validationErr.Error(),
		"",
		colors.Accent("Input Preview"),
		preview,
	}

	return strings.Join(lines, "\n")
}

func formatIntegrationInputParseError(features terminal.Features, raw string, parseErr error) string {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Input parsing failed"),
		"Tool input must be valid JSON.",
		"",
		colors.Accent("Why"),
		parseErr.Error(),
		"",
		colors.Accent("Input Preview"),
		previewJSON(strings.TrimSpace(raw)),
	}

	return strings.Join(lines, "\n")
}

func formatIntegrationInputObjectError(features terminal.Features, preview string) string {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Input shape is invalid"),
		"Tool input must be a JSON object at the top level.",
		"",
		colors.Accent("Input Preview"),
		preview,
	}

	return strings.Join(lines, "\n")
}

func normalizeMCPCallResult(result *mcp.CallToolResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}

	if result.StructuredContent != nil {
		if typed, ok := result.StructuredContent.(map[string]any); ok {
			return typed
		}

		return map[string]any{
			"value": result.StructuredContent,
		}
	}

	contentValues := mcpContentToValues(result.Content)
	if len(contentValues) == 1 {
		if typed, ok := contentValues[0].(map[string]any); ok {
			if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
				var decoded any
				if json.Unmarshal([]byte(text), &decoded) == nil {
					if object, ok := decoded.(map[string]any); ok {
						return object
					}
					return map[string]any{"value": decoded}
				}
				return map[string]any{"text": text}
			}
			return typed
		}
		if text, ok := contentValues[0].(string); ok {
			return map[string]any{"text": text}
		}
	}

	return map[string]any{
		"content": contentValues,
	}
}

func formatMCPToolResultError(features terminal.Features, server *magicmcpservers.MagicMcpServersGetOutput, tool *mcp.Tool, result *mcp.CallToolResult) error {
	colors := terminal.NewColorizer(features)
	lines := []string{
		colors.Warning("Tool call failed"),
		fmt.Sprintf("Integration: %s", colors.Bold(firstNonEmpty(optionalString(server.Name), server.Id))),
		fmt.Sprintf("Tool: %s", colors.Bold(firstNonEmpty(tool.Title, tool.Name))),
	}

	message := ""
	for _, item := range mcpContentToValues(result.Content) {
		switch typed := item.(type) {
		case map[string]any:
			if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
				message = text
				break
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				message = typed
				break
			}
		}
	}

	if strings.TrimSpace(message) != "" {
		lines = append(lines, "", colors.Accent("Server Message"), message)
	}

	if result.StructuredContent != nil {
		lines = append(lines, "", colors.Accent("Structured Error"), previewJSON(result.StructuredContent))
	}

	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

func writeIntegrationMachineValue(writer io.Writer, features terminal.Features, formatInput string, value any) error {
	format, err := output.ParseFormat(formatInput)
	if err != nil {
		return err
	}
	if format == output.FormatStructured {
		format = output.FormatJSON
	}

	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode response: %w", err)
	}

	return output.WriteResponse(writer, &fetch.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       body,
	}, output.RenderOptions{
		Format: format,
		Colors: features,
	})
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

func setupSessionRequiresUserAction(status, sessionURL string) bool {
	status = strings.TrimSpace(strings.ToLower(status))
	if strings.TrimSpace(sessionURL) == "" {
		return false
	}

	switch status {
	case "", "pending":
		return true
	default:
		return false
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

func attachSetupSessionProviderToMagicServer(
	runtime config.Runtime,
	sdk *metorial.MetorialSdk,
	setupSession *setupsessions.ProviderDeploymentsSetupSessionsGetOutput,
	server *magicmcpservers.MagicMcpServersCreateOutput,
) (*magicmcpserverprovider.MagicMcpServersProviderCreateOutput, error) {
	if setupSession == nil || server == nil {
		return nil, nil
	}

	body := map[string]any{}
	if setupSession.Deployment != nil && strings.TrimSpace(setupSession.Deployment.Id) != "" {
		body["provider_deployment_id"] = setupSession.Deployment.Id
	}
	if setupSession.Config != nil && strings.TrimSpace(setupSession.Config.Id) != "" {
		body["provider_config_id"] = setupSession.Config.Id
	}
	if setupSession.AuthConfig != nil && strings.TrimSpace(setupSession.AuthConfig.Id) != "" {
		body["provider_auth_config_id"] = setupSession.AuthConfig.Id
	}

	if len(body) == 0 {
		return nil, nil
	}

	sessionTemplateProvider, err := createMagicMcpServerProvider(runtime, sdk, server.Id, body)
	if err != nil {
		return nil, fmt.Errorf(
			"metorial: created integration %s but failed to attach its provider assignment: %w",
			integrationCreateIdentifier(server),
			err,
		)
	}

	return sessionTemplateProvider, nil
}

func listProviderTools(sdk *metorial.MetorialSdk, version any) ([]providertools.ProvidersToolsListOutputItems, error) {
	switch typed := version.(type) {
	case *providerlistings.ProviderListingsGetOutputProviderCurrentVersion:
		if typed == nil || strings.TrimSpace(typed.Id) == "" {
			return nil, nil
		}
		result, err := sdk.ProvidersTools.List(&endpoints.ProvidersToolsEndpointListParams{
			Limit:             float64Ptr(15),
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
			Limit:             float64Ptr(15),
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
		identifier := strings.TrimSpace(row.Alias)
		if identifier == "" {
			identifier = row.Id
		}

		name := identifier
		if strings.TrimSpace(row.Name) != "" {
			name = colors.Bold(row.Name) + "\n" + colors.Muted(identifier)
		}

		table.Rows = append(table.Rows, []string{
			name + "\n",
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
			colors.Accent("Identifier"),
			colors.Accent("Name"),
			colors.Accent("Publisher"),
			colors.Accent("Description"),
		},
		Features: features,
		MaxWidth: features.Width,
	}

	for _, row := range rows {
		table.Rows = append(table.Rows, []string{
			colors.Bold(row.Slug) + "\n" + colors.Muted(row.Id) + "\n",
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

func renderIntegrationTools(writer io.Writer, features terminal.Features, server *magicmcpservers.MagicMcpServersGetOutput, providersWithTools []integrationToolsProvider, liveTools []*mcp.Tool, tips []string) error {
	colors := terminal.NewColorizer(features)
	serverTitle := server.Id
	if name := optionalString(server.Name); name != "" {
		serverTitle = name
	}

	_, _ = fmt.Fprintln(writer, colors.Bold(serverTitle))
	_, _ = fmt.Fprintf(writer, "%s\n", colors.Muted(server.Id))

	if len(providersWithTools) == 0 {
		_, _ = fmt.Fprintln(writer, "(no linked providers)")
	} else {
		for index, providerWithTools := range providersWithTools {
			if index > 0 {
				_, _ = fmt.Fprintln(writer)
			}

			providerTitle := providerWithTools.Provider.Name
			if providerWithTools.Listing != nil {
				providerTitle = providerWithTools.Listing.Name
			}

			if !strings.EqualFold(strings.TrimSpace(providerTitle), strings.TrimSpace(serverTitle)) {
				_, _ = fmt.Fprintln(writer, colors.Accent(providerTitle))
			}
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
		}
	}

	if err := renderMCPToolsTable(writer, features, liveTools); err != nil {
		return err
	}

	return renderTips(writer, features, tips)
}

func renderSetupResult(writer io.Writer, features terminal.Features, setupSession *setupsessions.ProviderDeploymentsSetupSessionsGetOutput, listing *providerlistings.ProviderListingsGetOutput, provider *providers.ProvidersGetOutput, server *magicmcpservers.MagicMcpServersCreateOutput, sessionTemplateProvider *magicmcpserverprovider.MagicMcpServersProviderCreateOutput, tips []string) error {
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
	}

	if provider != nil {
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
			colors.Bold(tool.Key) + "\n" + colors.Muted(tool.Id) + "\n",
			tool.Name,
			optionalString(tool.Description),
		})
	}

	return table.Render(writer)
}

func renderMCPToolsTable(writer io.Writer, features terminal.Features, tools []*mcp.Tool) error {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Accent("Tools"))

	if len(tools) == 0 {
		_, _ = fmt.Fprintln(writer, "(no tools)")
		return nil
	}

	table := output.Table{
		Columns: []string{
			colors.Accent("Name"),
			colors.Accent("Title"),
			colors.Accent("Description"),
		},
		Features: features,
		MaxWidth: features.Width,
	}

	for _, tool := range tools {
		if tool == nil {
			continue
		}
		table.Rows = append(table.Rows, []string{
			tool.Name,
			firstNonEmpty(tool.Title, tool.Name),
			tool.Description,
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

func addPaginationFlags(command *cobra.Command, options *paginationOptions, label string) {
	command.Flags().Float64Var(&options.Limit, "limit", options.Limit, "Limit the number of "+label)
	command.Flags().StringVar(&options.After, "after", "", "Fetch "+label+" after this cursor")
	command.Flags().StringVar(&options.Before, "before", "", "Fetch "+label+" before this cursor")
	command.Flags().StringVar(&options.Cursor, "cursor", "", "Fetch "+label+" using an opaque cursor")
}

func applyPaginationOptionsToMagicMcpServers(params *endpoints.MagicMcpServersEndpointListParams, options paginationOptions) {
	params.Limit = float64Ptr(options.Limit)
	if after := strings.TrimSpace(options.After); after != "" {
		params.After = &after
	}
	if before := strings.TrimSpace(options.Before); before != "" {
		params.Before = &before
	}
	if cursor := strings.TrimSpace(options.Cursor); cursor != "" {
		params.Cursor = &cursor
	}
}

func applyPaginationOptionsToProviderListings(params *endpoints.ProviderListingsEndpointListParams, options paginationOptions) {
	params.Limit = float64Ptr(options.Limit)
	if after := strings.TrimSpace(options.After); after != "" {
		params.After = &after
	}
	if before := strings.TrimSpace(options.Before); before != "" {
		params.Before = &before
	}
	if cursor := strings.TrimSpace(options.Cursor); cursor != "" {
		params.Cursor = &cursor
	}
}

func paginationTipsForIntegrationList(args []string, options paginationOptions, items []magicmcpservers.MagicMcpServersListOutputItems, pagination magicmcpservers.MagicMcpServersListOutputPagination) []string {
	if len(items) == 0 {
		return nil
	}

	var tips []string
	search := strings.TrimSpace(optionalArg(args, 0))
	base := "metorial integrations list"
	if search != "" {
		base += " " + shellQuote(search)
	}
	base += paginationFlagString(options, true, true)

	if pagination.HasMoreAfter {
		tips = append(tips, fmt.Sprintf("%s --after %s", base, shellQuote(items[len(items)-1].Id)))
	}
	if pagination.HasMoreBefore {
		tips = append(tips, fmt.Sprintf("%s --before %s", base, shellQuote(items[0].Id)))
	}
	return tips
}

func paginationTipsForCatalogList(command *cobra.Command, args []string, options paginationOptions, items []providerlistings.ProviderListingsListOutputItems, pagination providerlistings.ProviderListingsListOutputPagination) []string {
	if len(items) == 0 {
		return nil
	}

	var tips []string
	search := strings.TrimSpace(optionalArg(args, 0))
	base := "metorial integrations"
	if command != nil && command.Parent() != nil && command.Parent().Name() == "catalog" {
		base += " catalog"
	}
	base += " list"
	if command != nil && command.Name() == "search" {
		base = strings.TrimSuffix(base, " list") + " search"
	}
	if search != "" {
		base += " " + shellQuote(search)
	}
	base += paginationFlagString(options, true, true)

	if pagination.HasMoreAfter {
		tips = append(tips, fmt.Sprintf("%s --after %s", base, shellQuote(items[len(items)-1].Id)))
	}
	if pagination.HasMoreBefore {
		tips = append(tips, fmt.Sprintf("%s --before %s", base, shellQuote(items[0].Id)))
	}
	return tips
}

func paginationFlagString(options paginationOptions, includeAfter bool, includeBefore bool) string {
	parts := make([]string, 0, 3)
	if options.Limit > 0 && options.Limit != 15 {
		parts = append(parts, fmt.Sprintf("--limit %s", formatPaginationLimit(options.Limit)))
	}
	if cursor := strings.TrimSpace(options.Cursor); cursor != "" {
		parts = append(parts, fmt.Sprintf("--cursor %s", shellQuote(cursor)))
	}
	if includeAfter {
		if before := strings.TrimSpace(options.Before); before != "" {
			parts = append(parts, fmt.Sprintf("--before %s", shellQuote(before)))
		}
	}
	if includeBefore {
		if after := strings.TrimSpace(options.After); after != "" {
			parts = append(parts, fmt.Sprintf("--after %s", shellQuote(after)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func formatPaginationLimit(limit float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", limit), "0"), ".")
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

func primaryServerAlias(endpoints []magicmcpservers.MagicMcpServersListOutputItemsEndpoints) string {
	if len(endpoints) == 0 {
		return ""
	}
	return strings.TrimSpace(endpoints[0].Alias)
}

func integrationListIdentifier(server magicmcpservers.MagicMcpServersListOutputItems) string {
	if len(server.Endpoints) > 0 && strings.TrimSpace(server.Endpoints[0].Alias) != "" {
		return strings.TrimSpace(server.Endpoints[0].Alias)
	}
	return server.Id
}

func integrationGetIdentifier(server *magicmcpservers.MagicMcpServersGetOutput) string {
	if server != nil && len(server.Endpoints) > 0 && strings.TrimSpace(server.Endpoints[0].Alias) != "" {
		return strings.TrimSpace(server.Endpoints[0].Alias)
	}
	if server == nil {
		return ""
	}
	return server.Id
}

func integrationCreateIdentifier(server *magicmcpservers.MagicMcpServersCreateOutput) string {
	if server != nil && len(server.Endpoints) > 0 && strings.TrimSpace(server.Endpoints[0].Alias) != "" {
		return strings.TrimSpace(server.Endpoints[0].Alias)
	}
	if server == nil {
		return ""
	}
	return server.Id
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
