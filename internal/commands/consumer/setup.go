package consumer

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

const setupFallbackTimeout = 30 * time.Minute

var setupPollInterval = 2 * time.Second

type setupOptions struct {
	AuthMethodID string
	Name         string
	Description  string
}

func newSetupCommands(env commandEnv) []*cobra.Command {
	return []*cobra.Command{
		newSetupCommand(env),
		newReconnectCommand(env),
	}
}

func newSetupCommand(env commandEnv) *cobra.Command {
	options := &setupOptions{}

	command := &cobra.Command{
		Use:   "setup [catalog-item-id]",
		Short: "Configure a new integration from your portal catalog",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			item, err := resolveSetupItem(env, client, commandutil.OptionalArg(args, 0))
			if err != nil {
				return err
			}
			if err := requireConfigurable(item); err != nil {
				return err
			}

			session, err := runSetupSession(env, command, client, item, options.AuthMethodID)
			if err != nil {
				return err
			}

			server, err := client.Deploy(item.ID, consumerapi.DeployInput{
				IntegrationSetupSessionID: session.ID,
				Name:                      stringPtrIfSet(options.Name),
				Description:               stringPtrIfSet(options.Description),
			})
			if err != nil {
				return fmt.Errorf("metorial: setup completed, but the integration could not be created: %w", err)
			}

			return writeSetupResult(env, command, "Integration Ready", item, session, server)
		},
	}

	command.Flags().StringVar(&options.AuthMethodID, "auth-method", "", "Auth method ID to use (defaults to OAuth when available)")
	command.Flags().StringVar(&options.Name, "name", "", "Name for the new integration")
	command.Flags().StringVar(&options.Description, "description", "", "Description for the new integration")
	return command
}

func newReconnectCommand(env commandEnv) *cobra.Command {
	options := &setupOptions{}

	command := &cobra.Command{
		Use:   "reconnect <catalog-item-id> [integration-id]",
		Short: "Re-run setup for an existing integration and keep its endpoint URL",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			item, err := client.GetProvider(args[0])
			if err != nil {
				return err
			}
			if err := requireConfigurable(item); err != nil {
				return err
			}

			server, err := resolveReconnectServer(client, item, commandutil.OptionalArg(args, 1))
			if err != nil {
				return err
			}

			session, err := runSetupSession(env, command, client, item, options.AuthMethodID)
			if err != nil {
				return err
			}

			reconnected, err := client.Reconnect(item.ID, server.ID, session.ID)
			if err != nil {
				return fmt.Errorf("metorial: setup completed, but %s could not be reconnected: %w", integrationIdentifier(server), err)
			}

			return writeSetupResult(env, command, "Integration Reconnected", item, session, reconnected)
		},
	}

	command.Flags().StringVar(&options.AuthMethodID, "auth-method", "", "Auth method ID to use (defaults to OAuth when available)")
	return command
}

func resolveSetupItem(env commandEnv, client *consumerapi.Client, catalogItemID string) (*consumerapi.Provider, error) {
	if catalogItemID = strings.TrimSpace(catalogItemID); catalogItemID != "" {
		return client.GetProvider(catalogItemID)
	}

	if !env.app().StdoutFeatures().IsTTY {
		return nil, fmt.Errorf("metorial: a catalog item is required. Run \"metorial catalog list\" to find one, then \"metorial setup <catalog-item-id>\"")
	}

	result, err := client.ListProviders(consumerapi.ListOptions{Limit: 100}, "", "")
	if err != nil {
		return nil, err
	}

	candidates := []consumerapi.Provider{}
	labels := []string{}
	for _, item := range result.Items {
		if item.Type != consumerapi.ProviderTypeProviderTemplate || item.Availability != consumerapi.AvailabilityAvailableNow {
			continue
		}
		candidates = append(candidates, item)
		labels = append(labels, fmt.Sprintf("%s (%s)", item.Name, item.ID))
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("metorial: no integrations are available to set up. Run \"metorial catalog list\" to see what your portal offers")
	}

	index, err := env.app().PromptSelect("Select an integration to set up", labels)
	if err != nil {
		return nil, err
	}

	return client.GetProvider(candidates[index].ID)
}

func requireConfigurable(item *consumerapi.Provider) error {
	if item.Availability == consumerapi.AvailabilityRequestAccess {
		return fmt.Errorf("metorial: you don't have access to %s yet. Run \"metorial request-access %s\" to ask for it", item.Name, item.ID)
	}
	if item.Type != consumerapi.ProviderTypeProviderTemplate {
		return fmt.Errorf("metorial: %s is a shared integration and is ready to use. Run \"%s\"", item.Name, catalogNextStep(*item))
	}
	return nil
}

func resolveReconnectServer(client *consumerapi.Client, item *consumerapi.Provider, integrationID string) (*consumerapi.MagicMcpServer, error) {
	if integrationID = strings.TrimSpace(integrationID); integrationID != "" {
		return client.GetMagicMcpServer(integrationID)
	}

	servers, err := client.ListMagicMcpServers(consumerapi.ListOptions{Limit: 50}, "", item.ProviderTemplate.ID)
	if err != nil {
		return nil, err
	}

	switch len(servers.Items) {
	case 0:
		return nil, fmt.Errorf("metorial: you have no integrations for %s yet. Run \"metorial setup %s\" to create one", item.Name, item.ID)
	case 1:
		return &servers.Items[0], nil
	default:
		ids := make([]string, 0, len(servers.Items))
		for index := range servers.Items {
			ids = append(ids, integrationIdentifier(&servers.Items[index]))
		}
		return nil, fmt.Errorf("metorial: you have several integrations for %s (%s). Pass the one to reconnect: \"metorial reconnect %s <integration-id>\"", item.Name, strings.Join(ids, ", "), item.ID)
	}
}

func runSetupSession(env commandEnv, command *cobra.Command, client *consumerapi.Client, item *consumerapi.Provider, authMethodID string) (*consumerapi.SetupSession, error) {
	if strings.TrimSpace(authMethodID) == "" {
		authMethodID = preferredAuthMethodID(item)
	}

	session, err := client.StartSetup(item.ID, authMethodID)
	if err != nil {
		return nil, err
	}
	if session.Status == consumerapi.SetupStatusSuccessful {
		return session, nil
	}

	structured, err := env.ctx.Structured()
	if err != nil {
		return nil, err
	}

	writer := command.ErrOrStderr()
	features := env.app().StderrFeatures()
	if structured {
		writer = command.OutOrStdout()
		features = env.app().StdoutFeatures()
	}

	if strings.TrimSpace(session.URL) != "" {
		if err := renderSetupPrompt(writer, features, item, session); err != nil {
			return nil, err
		}
	}

	return waitForSetupSession(writer, features, client, item.ID, session)
}

func preferredAuthMethodID(item *consumerapi.Provider) string {
	for _, method := range item.AuthMethods {
		if method.Type == "oauth" {
			return method.ID
		}
	}
	return ""
}

func renderSetupPrompt(writer io.Writer, features terminal.Features, item *consumerapi.Provider, session *consumerapi.SetupSession) error {
	colors := terminal.NewColorizer(features)
	browserOpenSupported := browser.Supported()

	summary := fmt.Sprintf("%s\n%s", colors.Notice("Open this URL to finish setting up "+item.Name), colors.Bold(session.URL))
	if browserOpenSupported {
		summary = fmt.Sprintf("%s\n%s", colors.Success("Opened setup for "+item.Name+" in your browser"), colors.Bold(session.URL))
	}

	lines := []string{
		summary,
		fmt.Sprintf("%s %s", colors.Muted("Setup session:"), colors.Bold(session.ID)),
	}
	if !session.ExpiresAt.IsZero() {
		lines = append(lines, fmt.Sprintf("%s %s", colors.Muted("Expires:"), session.ExpiresAt.Local().Format("2006-01-02 15:04:05")))
	}

	if err := output.RenderBox(writer, lines, output.BoxOptions{
		MaxWidth: features.Width,
		Unicode:  features.HasUnicode,
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(writer)

	if browserOpenSupported {
		_ = browser.Open(session.URL)
	}
	return nil
}

func waitForSetupSession(writer io.Writer, features terminal.Features, client *consumerapi.Client, catalogItemID string, session *consumerapi.SetupSession) (*consumerapi.SetupSession, error) {
	deadline := session.ExpiresAt
	if deadline.IsZero() {
		deadline = time.Now().Add(setupFallbackTimeout)
	}

	spinner := terminal.NewSpinner(writer, features, "Waiting for setup to complete")
	spinner.Start()
	defer spinner.Stop()

	for {
		current, err := client.GetSetup(catalogItemID, session.ID)
		if err != nil {
			return nil, err
		}

		switch current.Status {
		case consumerapi.SetupStatusSuccessful:
			return current, nil
		case consumerapi.SetupStatusExpired:
			return nil, fmt.Errorf("metorial: setup session %s expired before it was completed", current.ID)
		case consumerapi.SetupStatusArchived, consumerapi.SetupStatusDeleted:
			return nil, fmt.Errorf("metorial: setup session %s was cancelled before it was completed", current.ID)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("metorial: setup session %s expired before it was completed", current.ID)
		}

		time.Sleep(setupPollInterval)
	}
}

func writeSetupResult(env commandEnv, command *cobra.Command, title string, item *consumerapi.Provider, session *consumerapi.SetupSession, server *consumerapi.MagicMcpServer) error {
	tips := append([]string{fmt.Sprintf("metorial get %s", integrationIdentifier(server))}, integrationTips(server, true)...)

	return env.ctx.Write(command, map[string]any{
		"item":          item,
		"setup_session": session,
		"integration":   server,
		"tips":          tips,
	}, func(writer io.Writer, features terminal.Features) error {
		colors := terminal.NewColorizer(features)
		_, _ = fmt.Fprintln(writer, colors.Success(title))
		_, _ = fmt.Fprintln(writer)
		commandutil.RenderTitle(writer, features, integrationTitle(server), server.ID)

		if err := output.RenderDataList(writer, []output.DataListItem{
			{Label: "Status", Value: server.Status},
			{Label: "Catalog Item", Value: fmt.Sprintf("%s (%s)", item.Name, item.ID)},
		}); err != nil {
			return err
		}
		if err := renderServerEndpoints(writer, features, server); err != nil {
			return err
		}
		return commandutil.RenderTips(writer, features, tips)
	})
}
