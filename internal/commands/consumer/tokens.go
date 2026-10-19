package consumer

import (
	"fmt"
	"io"

	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

type tokenListRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Integration string `json:"integration"`
	CreatedAt   string `json:"created_at"`
}

func newTokensCommand(env commandEnv) *cobra.Command {
	command := &cobra.Command{
		Use:   "tokens",
		Short: "Manage the long-lived MCP tokens created for your integrations",
	}
	command.AddCommand(newTokensListCommand(env), newTokensRevokeCommand(env))
	return command
}

func newTokensListCommand(env commandEnv) *cobra.Command {
	options := paginationOptions{Limit: defaultPageLimit}

	command := &cobra.Command{
		Use:   "list",
		Short: "List your active MCP tokens",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			result, err := client.ListMagicMcpTokens(options.list())
			if err != nil {
				return err
			}

			rows := make([]tokenListRow, 0, len(result.Items))
			for _, token := range result.Items {
				rows = append(rows, tokenListRow{
					ID:          token.ID,
					Name:        optionalString(token.Name),
					Integration: tokenIntegrationLabel(token),
					CreatedAt:   token.CreatedAt.Format("2006-01-02 15:04"),
				})
			}

			tips := []string{}
			if len(rows) > 0 {
				tips = append(tips, fmt.Sprintf("metorial tokens revoke %s", shellQuote(rows[0].ID)))
			}
			firstID, lastID := "", ""
			if len(rows) > 0 {
				firstID, lastID = rows[0].ID, rows[len(rows)-1].ID
			}
			tips = append(tips, paginationTips("metorial tokens list", options, firstID, lastID, result.Pagination)...)

			return env.write(command, map[string]any{
				"items":      result.Items,
				"pagination": result.Pagination,
				"tips":       tips,
			}, func(writer io.Writer, features terminal.Features) error {
				return renderTokenList(writer, features, rows, tips)
			})
		},
	}
	addPaginationFlags(command, &options, "tokens")
	return command
}

func newTokensRevokeCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:     "revoke <token-id>",
		Aliases: []string{"delete"},
		Short:   "Revoke an MCP token",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := env.client()
			if err != nil {
				return err
			}

			token, err := client.DeleteMagicMcpToken(args[0])
			if err != nil {
				return err
			}

			return env.write(command, token, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Success("Revoked token"), token.ID)
				return nil
			})
		},
	}
}

func tokenIntegrationLabel(token consumerapi.MagicMcpToken) string {
	if token.Server == nil {
		return ""
	}
	return firstNonEmpty(optionalString(token.Server.Name), token.Server.ID)
}

func renderTokenList(writer io.Writer, features terminal.Features, rows []tokenListRow, tips []string) error {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(writer, "No active tokens. `metorial install` creates one for each client you install into.")
		return nil
	}

	colors := terminal.NewColorizer(features)
	table := output.Table{
		Columns: []string{
			colors.Accent("ID"),
			colors.Accent("Name"),
			colors.Accent("Integration"),
			colors.Accent("Created"),
		},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, row := range rows {
		table.Rows = append(table.Rows, []string{row.ID, row.Name, row.Integration, row.CreatedAt})
	}
	if err := table.Render(writer); err != nil {
		return err
	}

	return renderTips(writer, features, tips)
}
