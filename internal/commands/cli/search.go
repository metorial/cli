package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

type searchResult struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	score       int
}

func newSearchCommand(ctx commandutil.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search available commands by name or description",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			query := strings.TrimSpace(strings.Join(args, " "))
			results := searchCommands(command.Root(), query)

			format, err := output.ParseFormat(ctx.Options.Format)
			if err != nil {
				return err
			}

			if format != output.FormatStructured {
				return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, map[string]any{
					"object":   "cli.search",
					"query":    query,
					"commands": results,
				})
			}

			colors := terminal.NewColorizer(ctx.App.StdoutFeatures())
			if len(results) == 0 {
				_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Bold("No Matching Commands"))
				_, _ = fmt.Fprintln(command.OutOrStdout())
				_, _ = fmt.Fprintf(command.OutOrStdout(), "%s\n", colors.Muted(fmt.Sprintf("Nothing matches %q. Run `%s --help` to see all commands.", query, command.Root().Name())))
				return nil
			}

			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Bold("Matching Commands"))
			_, _ = fmt.Fprintln(command.OutOrStdout(), colors.Muted(fmt.Sprintf("%d %s %q.", len(results), matchPhrase(len(results)), query)))
			_, _ = fmt.Fprintln(command.OutOrStdout())

			table := output.Table{
				Columns: []string{
					colors.Accent("Command"),
					colors.Accent("Description"),
				},
				Features: ctx.App.StdoutFeatures(),
				MaxWidth: ctx.App.StdoutFeatures().Width,
			}
			for _, result := range results {
				table.Rows = append(table.Rows, []string{colors.Bold(result.Command), result.Description})
			}

			if err := table.Render(command.OutOrStdout()); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(command.OutOrStdout())
			_, _ = fmt.Fprintf(command.OutOrStdout(), "%s `%s --help`\n", colors.Notice("Learn more about a command with"), results[0].Command)
			return nil
		},
	}
}

func searchCommands(root *cobra.Command, query string) []searchResult {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return []searchResult{}
	}

	results := []searchResult{}
	var visit func(command *cobra.Command)
	visit = func(command *cobra.Command) {
		for _, child := range command.Commands() {
			if !child.IsAvailableCommand() || child.Hidden {
				continue
			}

			if child.Runnable() {
				if score, ok := scoreCommand(child, terms); ok {
					results = append(results, searchResult{
						Command:     child.CommandPath(),
						Description: child.Short,
						score:       score,
					})
				}
			}

			visit(child)
		}
	}
	visit(root)

	sort.SliceStable(results, func(left, right int) bool {
		if results[left].score != results[right].score {
			return results[left].score > results[right].score
		}
		return results[left].Command < results[right].Command
	})

	return results
}

// Every term must appear in the command path, aliases, or description; terms
// found in the path or aliases rank above description-only matches.
func scoreCommand(command *cobra.Command, terms []string) (int, bool) {
	name := strings.ToLower(strings.TrimPrefix(command.CommandPath(), command.Root().Name()) + " " + strings.Join(command.Aliases, " "))
	description := strings.ToLower(command.Short)

	score := 0
	for _, term := range terms {
		switch {
		case strings.Contains(name, term):
			score += 2
		case strings.Contains(description, term):
			score++
		default:
			return 0, false
		}
	}

	return score, true
}

func matchPhrase(count int) string {
	if count == 1 {
		return "command matches"
	}
	return "commands match"
}
