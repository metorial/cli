package skills

import (
	"fmt"
	"io"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/skillsapi"
	"github.com/metorial/cli/internal/skillsync"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

func renderFileTable(writer io.Writer, features terminal.Features, files []skillsapi.File) error {
	if len(files) == 0 {
		_, _ = fmt.Fprintln(writer, "This skill has no files.")
		return nil
	}

	colors := terminal.NewColorizer(features)
	table := output.Table{
		Columns:  []string{colors.Accent("Path"), colors.Accent("Kind"), colors.Accent("Size")},
		Features: features,
		MaxWidth: features.Width,
	}
	for _, file := range files {
		table.Rows = append(table.Rows, []string{file.Path, file.Kind, commandutil.HumanBytes(file.Size)})
	}

	return table.Render(writer)
}

func renderChanges(writer io.Writer, features terminal.Features, changes []skillsync.Change) {
	colors := terminal.NewColorizer(features)

	for _, change := range changes {
		label := colors.Success("added   ")
		switch change.Kind {
		case skillsync.ChangeModified:
			label = colors.Notice("modified")
		case skillsync.ChangeDeleted:
			label = colors.Warning("deleted ")
		}

		_, _ = fmt.Fprintf(writer, "  %s %s\n", label, change.Path)
	}
}

func binaryName(command *cobra.Command) string {
	return command.Root().Name()
}

func skillReference(skill skillsapi.Skill) string {
	return commandutil.FirstNonEmpty(commandutil.OptionalString(skill.Slug), skill.ID)
}
