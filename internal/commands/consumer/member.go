package consumer

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

// NewMemberCommand exposes the consumer setup and MCP transport using an admin CLI login.
func NewMemberCommand(ctx commandutil.Context) *cobra.Command {
	env := commandEnv{ctx: ctx, member: true}
	command := &cobra.Command{Use: "personal", Short: "Connect integrations as your member consumer"}
	command.AddCommand(
		newMemberSetupCommand(env),
		newMemberSetupStatusCommand(env, false),
		newMemberSetupStatusCommand(env, true),
		newMemberFinishCommand(env),
		newToolsCommand(env),
	)
	return command
}

func newMemberSetupCommand(env commandEnv) *cobra.Command {
	var authMethod string
	var noBrowser bool
	command := &cobra.Command{
		Use:   "setup <integration-id>",
		Short: "Open setup for a personal connection to an existing integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if _, err := env.ctx.Structured(); err != nil {
				return err
			}
			runtime, err := env.ctx.ResolveRuntime()
			if err != nil {
				return err
			}
			client, catalogItemID, err := consumerapi.NewMember(runtime, args[0])
			if err != nil {
				return err
			}
			if catalogItemID == "" {
				return fmt.Errorf("metorial: the integration has no personal catalog item")
			}
			item, err := client.GetProvider(catalogItemID)
			if err != nil {
				return err
			}
			if err := requireConfigurable(item); err != nil {
				return err
			}
			session, err := startSetupSession(client, item, authMethod)
			if err != nil {
				return err
			}

			resume := command.Parent().CommandPath() + " "
			ids := commandutil.ShellQuote(item.ID) + " " + commandutil.ShellQuote(session.ID)
			fmt.Fprintf(command.ErrOrStderr(), "Check with %sstatus %s or %swait %s. When ready, run %sfinish %s. Reuse the same profile and instance.\n",
				resume, ids, resume, ids, resume, ids)
			if err := writeMemberSetup(command, env, item.ID, session); err != nil {
				return err
			}

			link, err := url.Parse(session.URL)
			if !noBrowser && session.Status == consumerapi.SetupStatusPending && err == nil &&
				link.Host != "" && link.User == nil && (link.Scheme == "https" || link.Scheme == "http") && browser.Supported() {
				_ = browser.Open(session.URL)
			}
			return nil
		},
	}
	command.Flags().StringVar(&authMethod, "auth-method", "", "Auth method ID to use (defaults to OAuth when available)")
	command.Flags().BoolVar(&noBrowser, "no-browser", false, "Print the setup link without opening a browser")
	return command
}

func newMemberSetupStatusCommand(env commandEnv, wait bool) *cobra.Command {
	name := "status"
	short := "Read a personal setup session"
	if wait {
		name = "wait"
		short = "Wait for a personal setup session to finish"
	}
	var timeout time.Duration
	command := &cobra.Command{
		Use:   name + " <catalog-item-id> <setup-session-id>",
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if wait && timeout <= 0 {
				return fmt.Errorf("metorial: --timeout must be positive")
			}
			client, err := env.client()
			if err != nil {
				return err
			}

			ctx := commandContext(command)
			if wait {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			session, err := getSetupWithContext(ctx, client, args[0], args[1])
			if err != nil {
				return err
			}
			if wait && session.Status == consumerapi.SetupStatusPending {
				session, err = waitForSetupSessionUntil(ctx, command.ErrOrStderr(), env.app().StderrFeatures(), client, args[0], session)
				if err != nil {
					return err
				}
			}
			if wait && session.Status != consumerapi.SetupStatusSuccessful {
				return fmt.Errorf("metorial: setup session %s is %s", session.ID, session.Status)
			}
			return writeMemberSetup(command, env, args[0], session)
		},
	}
	if wait {
		command.Flags().DurationVar(&timeout, "timeout", time.Minute, "Maximum time to wait for authorization")
	}
	return command
}

func writeMemberSetup(command *cobra.Command, env commandEnv, catalogItemID string, session *consumerapi.SetupSession) error {
	return env.ctx.Write(command, map[string]any{
		"catalog_item_id": catalogItemID,
		"setup_session":   session,
	}, func(writer io.Writer, features terminal.Features) error {
		return output.RenderDataList(writer, []output.DataListItem{
			{Label: "Catalog item", Value: catalogItemID},
			{Label: "Setup session", Value: session.ID},
			{Label: "Status", Value: string(session.Status)},
			{Label: "Setup URL", Value: session.URL},
		})
	})
}

func newMemberFinishCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "finish <catalog-item-id> <setup-session-id>",
		Short: "Deploy a completed personal integration and verify its tools",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if _, err := env.ctx.Structured(); err != nil {
				return err
			}
			client, err := env.client()
			if err != nil {
				return err
			}

			session, err := client.GetSetup(args[0], args[1])
			if err != nil {
				return err
			}
			if session.Status != consumerapi.SetupStatusSuccessful {
				return fmt.Errorf("metorial: setup session %s is %s", session.ID, session.Status)
			}
			server, err := client.Deploy(args[0], consumerapi.DeployInput{IntegrationSetupSessionID: session.ID})
			if err != nil {
				return err
			}

			ctx := commandContext(command)
			count, verifyErr := verifyMemberIntegration(ctx, client, server)
			result := map[string]any{"integration": server, "verified": verifyErr == nil}
			if verifyErr == nil {
				result["tool_count"] = count
			}
			if err := env.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				commandutil.RenderTitle(writer, features, integrationTitle(server), server.ID)
				if verifyErr == nil {
					fmt.Fprintf(writer, "Verified %d tools.\n", count)
				}
				return renderServerEndpoints(writer, features, server)
			}); err != nil {
				return err
			}
			if verifyErr != nil {
				return fmt.Errorf("metorial: server %s is ready but verification failed. Retry with %s tools %s: %w",
					server.ID, command.Parent().CommandPath(), server.ID, verifyErr)
			}
			return nil
		},
	}
}

func verifyMemberIntegration(ctx context.Context, client *consumerapi.Client, server *consumerapi.MagicMcpServer) (int, error) {
	mcpClient, err := connectMagicMCP(ctx, client, server)
	if err != nil {
		return 0, err
	}
	defer mcpClient.Close()
	tools, err := mcpClient.ListTools(ctx)
	return len(tools), err
}
