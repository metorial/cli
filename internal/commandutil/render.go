package commandutil

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

func (c Context) Write(command *cobra.Command, value any, render func(io.Writer, terminal.Features) error) error {
	structured, err := c.Structured()
	if err != nil {
		return err
	}

	features := c.App.StdoutFeatures()
	if !structured {
		return WriteValue(command.OutOrStdout(), features, c.Options.Format, value)
	}

	return render(command.OutOrStdout(), features)
}

func (c Context) Structured() (bool, error) {
	format, err := output.ParseFormat(c.Options.Format)
	if err != nil {
		return false, err
	}

	return format == output.FormatStructured, nil
}

func RenderTitle(writer io.Writer, features terminal.Features, title, subtitle string) {
	colors := terminal.NewColorizer(features)
	_, _ = fmt.Fprintln(writer, colors.Bold(title))
	if strings.TrimSpace(subtitle) != "" {
		_, _ = fmt.Fprintln(writer, colors.Muted(subtitle))
	}
	_, _ = fmt.Fprintln(writer)
}

func RenderTips(writer io.Writer, features terminal.Features, tips []string) error {
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

func OptionalArg(args []string, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	return args[index]
}

func OptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func ShellQuote(value string) string {
	if shellSafe.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
