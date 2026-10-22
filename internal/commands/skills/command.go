package skills

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/metorial/cli/internal/skillsapi"
	"github.com/metorial/cli/internal/skillsync"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

const defaultPageLimit = 15

type env struct {
	ctx commandutil.Context
}

func NewCommand(ctx commandutil.Context) *cobra.Command {
	e := env{ctx: ctx}

	command := &cobra.Command{
		Use:     "skills",
		Aliases: []string{"skill"},
		Short:   "Create, edit and sync your Metorial skills",
		Long: strings.TrimSpace(`
Work with Metorial skills from your terminal.

Pull a skill into a local folder, edit it with any tool, and push the changes back.

Examples:
  metorial skills list
  metorial skills pull release-notes ./release-notes
  metorial skills status ./release-notes
  metorial skills push ./release-notes
`),
	}

	command.AddCommand(
		e.newListCommand(),
		e.newGetCommand(),
		e.newCreateCommand(),
		e.newFilesCommand(),
	)
	command.AddCommand(e.localCommands()...)

	return command
}

func Attach(root *cobra.Command, ctx commandutil.Context) error {
	skills := resourcecmd.CommandByName(root, "skills")
	if skills == nil {
		return errors.New("metorial: the generated skills command is missing")
	}

	e := env{ctx: ctx}
	for _, command := range e.localCommands() {
		commandutil.ConfigureCommand(command)
		commandutil.SetCommandCategory(command, commandutil.CommandCategoryResource)
		skills.AddCommand(command)
	}

	return nil
}

func (e env) localCommands() []*cobra.Command {
	return []*cobra.Command{
		e.newPullCommand(),
		e.newPushCommand(),
		e.newStatusCommand(),
	}
}

func (e env) client() (*skillsapi.Client, error) {
	runtime, err := e.ctx.ResolveRuntime()
	if err != nil {
		return nil, err
	}

	return skillsapi.New(runtime)
}

func (e env) newListCommand() *cobra.Command {
	var search string
	limit := float64(defaultPageLimit)

	command := &cobra.Command{
		Use:   "list",
		Short: "List the skills you have access to",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			result, err := client.ListSkills(skillsapi.ListOptions{Limit: limit}, search)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				if len(result.Items) == 0 {
					_, _ = fmt.Fprintln(writer, "No skills found.")
					return nil
				}

				colors := terminal.NewColorizer(features)
				table := output.Table{
					Columns:  []string{colors.Accent("ID"), colors.Accent("Slug"), colors.Accent("Name")},
					Features: features,
					MaxWidth: features.Width,
				}
				for _, skill := range result.Items {
					table.Rows = append(table.Rows, []string{skill.ID, commandutil.OptionalString(skill.Slug), skill.Name})
				}
				if err := table.Render(writer); err != nil {
					return err
				}

				return commandutil.RenderTips(writer, features, []string{
					fmt.Sprintf("%s skills pull %s", binaryName(command), commandutil.ShellQuote(skillReference(result.Items[0]))),
				})
			})
		},
	}

	command.Flags().StringVar(&search, "search", "", "Filter skills by name or description")
	command.Flags().Float64Var(&limit, "limit", limit, "Limit the number of skills")
	return command
}

func (e env) newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <skill>",
		Short: "Show a skill and its files",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			files, err := client.ListFiles(skill.ID)
			if err != nil {
				return err
			}

			value := map[string]any{"skill": skill, "files": files.Items}
			return e.ctx.Write(command, value, func(writer io.Writer, features terminal.Features) error {
				commandutil.RenderTitle(writer, features, skill.Name, commandutil.FirstNonEmpty(commandutil.OptionalString(skill.Description), skill.ID))
				return renderFileTable(writer, features, files.Items)
			})
		},
	}
}

func (e env) newCreateCommand() *cobra.Command {
	input := skillsapi.CreateSkillInput{}

	command := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			input.Name = args[0]
			skill, err := client.CreateSkill(input)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, skill, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintf(writer, "%s %s (%s)\n", colors.Success("Created skill"), skill.Name, skill.ID)
				return commandutil.RenderTips(writer, features, []string{
					fmt.Sprintf("%s skills pull %s", binaryName(command), commandutil.ShellQuote(skillReference(*skill))),
				})
			})
		},
	}

	command.Flags().StringVar(&input.Description, "description", "", "A short description of the skill")
	command.Flags().StringVar(&input.TemplateID, "template", "", "Create the skill from this skill template")
	return command
}

func (e env) newPullCommand() *cobra.Command {
	options := skillsync.PullOptions{}

	command := &cobra.Command{
		Use:   "pull <skill> [dir]",
		Short: "Download a skill into a local folder",
		Long: strings.TrimSpace(`
Download every file of a skill into a local folder. The folder defaults to the
skill's slug. Running pull again in the same folder updates it, as long as it has
no local changes (use --force to discard them).
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			dir := commandutil.FirstNonEmpty(commandutil.OptionalArg(args, 1), skillReference(*skill))
			result, err := skillsync.Pull(client, skill, dir, options)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintf(writer, "%s %d file(s) of %s into %s\n", colors.Success("Pulled"), len(result.Written), skill.Name, dir)
				if len(result.Removed) > 0 {
					_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Muted("Removed:"), strings.Join(result.Removed, ", "))
				}
				if len(result.Skipped) > 0 {
					_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Warning("Skipped ignored paths:"), strings.Join(result.Skipped, ", "))
				}
				return commandutil.RenderTips(writer, features, []string{
					fmt.Sprintf("%s skills status %s", binaryName(command), commandutil.ShellQuote(dir)),
					fmt.Sprintf("%s skills push %s", binaryName(command), commandutil.ShellQuote(dir)),
				})
			})
		},
	}

	command.Flags().BoolVar(&options.Force, "force", false, "Overwrite local changes and non-empty folders")
	return command
}

func (e env) newPushCommand() *cobra.Command {
	options := skillsync.PushOptions{}

	command := &cobra.Command{
		Use:   "push [dir]",
		Short: "Upload local changes of a pulled skill",
		Long: strings.TrimSpace(`
Upload the files you added, changed or deleted since the last pull or push.

Before sending each batch of up to 100 changes, push checks that none of the files
it touches changed remotely since your last pull, and stops if they did. Pull to
update your copy first, or pass --force to overwrite the remote changes. Batches are
not atomic: if the server rejects a change, the changes before it stay applied and
are recorded locally, so status shows what is left.

Push never deletes /SKILL.md.

These paths are never pushed or pulled: .git, .metorial, .DS_Store, node_modules,
.idea, .vscode, .env, .env.*, *.swp, *~ and .metorial-* temporary files. Add up to
30 more patterns (one per line, matched per path segment, or against the whole path
when they contain a /) to a .metorialignore file in the skill folder. The
.metorialignore file is itself synced.
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			dir := commandutil.FirstNonEmpty(commandutil.OptionalArg(args, 0), ".")
			result, err := skillsync.Push(client, dir, options)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				if len(result.Changes) == 0 {
					_, _ = fmt.Fprintln(writer, "Nothing to push.")
					return nil
				}

				verb := colors.Success("Pushed")
				if result.DryRun {
					verb = colors.Notice("Would push")
				}
				_, _ = fmt.Fprintf(writer, "%s %d change(s)\n", verb, len(result.Changes))
				renderChanges(writer, features, result.Changes)
				return nil
			})
		},
	}

	command.Flags().BoolVar(&options.Force, "force", false, "Overwrite changes made remotely since the last pull")
	command.Flags().BoolVar(&options.DryRun, "dry-run", false, "Show what would be pushed without changing anything")
	return command
}

func (e env) newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [dir]",
		Short: "Show local and remote changes of a pulled skill",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			dir := commandutil.FirstNonEmpty(commandutil.OptionalArg(args, 0), ".")
			status, err := skillsync.GetStatus(client, dir)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, status, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				if status.Clean() {
					_, _ = fmt.Fprintln(writer, "Up to date with the remote skill.")
					return nil
				}

				if len(status.Local) > 0 {
					_, _ = fmt.Fprintln(writer, colors.Bold("Local changes"))
					renderChanges(writer, features, status.Local)
				}
				if len(status.Remote) > 0 {
					_, _ = fmt.Fprintln(writer, colors.Bold("Remote changes"))
					renderChanges(writer, features, status.Remote)
				}

				reference := commandutil.ShellQuote(commandutil.FirstNonEmpty(status.SkillSlug, status.SkillID))
				pull := fmt.Sprintf("%s skills pull %s %s", binaryName(command), reference, commandutil.ShellQuote(dir))
				push := fmt.Sprintf("%s skills push %s", binaryName(command), commandutil.ShellQuote(dir))

				tips := []string{}
				switch {
				case status.Conflicted():
					_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Warning("Changed on both sides:"), strings.Join(status.Conflicts(), ", "))
					pull += " --force"
					push += " --force"
					width := max(len(pull), len(push))
					tips = append(tips,
						fmt.Sprintf("%-*s  # discard local changes", width, pull),
						fmt.Sprintf("%-*s  # overwrite remote changes", width, push),
					)
				case len(status.Local) > 0 && len(status.Remote) > 0:
					tips = append(tips, push, pull)
				case len(status.Remote) > 0:
					tips = append(tips, pull)
				default:
					tips = append(tips, push)
				}
				return commandutil.RenderTips(writer, features, tips)
			})
		},
	}
}

func (e env) newFilesCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "files",
		Short: "Read and edit individual files of a skill",
	}

	command.AddCommand(
		e.newFilesListCommand(),
		e.newFilesCatCommand(),
		e.newFilesPutCommand(),
		e.newFilesRemoveCommand(),
		e.newFilesMoveCommand(),
	)

	return command
}

func (e env) newFilesListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list <skill>",
		Aliases: []string{"ls"},
		Short:   "List the files of a skill",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			files, err := client.ListFiles(skill.ID)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, files, func(writer io.Writer, features terminal.Features) error {
				return renderFileTable(writer, features, files.Items)
			})
		},
	}
}

func (e env) newFilesCatCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cat <skill> <path>",
		Short: "Print the content of a skill file",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			file, content, err := client.ReadFile(skill.ID, args[1])
			if err != nil {
				return err
			}

			_, err = command.OutOrStdout().Write(skillsapi.RestoreTrailingNewline(file.Kind, content))
			return err
		},
	}
}

func (e env) newFilesPutCommand() *cobra.Command {
	var source string
	var ifMatch string

	command := &cobra.Command{
		Use:   "put <skill> <path>",
		Short: "Create or replace a skill file from a local file or stdin",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			content, err := readSource(command, source)
			if err != nil {
				return err
			}

			isText, err := skillsapi.IsText(args[1], content)
			if err != nil {
				return err
			}

			change := skillsapi.ChangeInput{Path: args[1], IfMatch: skillsapi.ParseIfMatch(ifMatch)}
			if isText {
				text := string(content)
				change.Content = &text
			} else {
				uploaded, err := client.UploadFile(path.Base(args[1]), content)
				if err != nil {
					return err
				}
				change.FileID = uploaded.ID
			}

			result, err := client.PutFile(skill.ID, change)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Success("Wrote"), result.Path)
				return nil
			})
		},
	}

	command.Flags().StringVarP(&source, "file", "f", "", "Read the content from this local file instead of stdin")
	command.Flags().StringVar(&ifMatch, "if-match", "", "Only write if the remote file still has this etag, or \"absent\" if it must not exist yet")
	return command
}

func (e env) newFilesRemoveCommand() *cobra.Command {
	var ifMatch string

	command := &cobra.Command{
		Use:     "remove <skill> <path>",
		Aliases: []string{"rm"},
		Short:   "Delete a skill file",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			etag, err := skillsapi.ParseEtag(ifMatch)
			if err != nil {
				return err
			}

			result, err := client.DeleteFile(skill.ID, args[1], etag)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Success("Deleted"), result.Path)
				return nil
			})
		},
	}

	command.Flags().StringVar(&ifMatch, "if-match", "", "Only delete if the remote file still has this etag")
	return command
}

func (e env) newFilesMoveCommand() *cobra.Command {
	var ifMatch string

	command := &cobra.Command{
		Use:     "move <skill> <from> <to>",
		Aliases: []string{"mv"},
		Short:   "Move or rename a skill file",
		Args:    cobra.ExactArgs(3),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := e.client()
			if err != nil {
				return err
			}

			skill, err := client.ResolveSkill(args[0])
			if err != nil {
				return err
			}

			etag, err := skillsapi.ParseEtag(ifMatch)
			if err != nil {
				return err
			}

			change := skillsapi.ChangeInput{Path: args[1], ToPath: args[2]}
			if etag != "" {
				change.IfMatch = skillsapi.MatchEtag(etag)
			}

			result, err := client.MoveFile(skill.ID, change)
			if err != nil {
				return err
			}

			return e.ctx.Write(command, result, func(writer io.Writer, features terminal.Features) error {
				colors := terminal.NewColorizer(features)
				from := commandutil.FirstNonEmpty(commandutil.OptionalString(result.PreviousPath), args[1])
				_, _ = fmt.Fprintf(writer, "%s %s -> %s\n", colors.Success("Moved"), from, result.Path)
				return nil
			})
		},
	}

	command.Flags().StringVar(&ifMatch, "if-match", "", "Only move if the remote file still has this etag")
	return command
}

func readSource(command *cobra.Command, source string) ([]byte, error) {
	if source != "" {
		content, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read %s: %w", source, err)
		}
		return content, nil
	}

	content, err := io.ReadAll(command.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to read stdin: %w", err)
	}
	return content, nil
}
