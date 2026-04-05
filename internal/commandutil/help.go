package commandutil

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

const METORIAL_1 = ` /$$      /$$             /$$                         /$$           /$$
| $$$    /$$$            | $$                        |__/          | $$
| $$$$  /$$$$  /$$$$$$  /$$$$$$    /$$$$$$   /$$$$$$  /$$  /$$$$$$ | $$
| $$ $$/$$ $$ /$$__  $$|_  $$_/   /$$__  $$ /$$__  $$| $$ |____  $$| $$
| $$  $$$| $$| $$$$$$$$  | $$    | $$  \ $$| $$  \__/| $$  /$$$$$$$| $$
| $$\  $ | $$| $$_____/  | $$ /$$| $$  | $$| $$      | $$ /$$__  $$| $$
| $$ \/  | $$|  $$$$$$$  |  $$$$/|  $$$$$$/| $$      | $$|  $$$$$$$| $$
|__/     |__/ \_______/   \___/   \______/ |__/      |__/ \_______/|__/`

const METORIAL_2 = ` _  _  ____  ____  __  ____  __   __   __   
( \/ )(  __)(_  _)/  \(  _ \(  ) / _\ (  )  
/ \/ \ ) _)   )( (  O ))   / )( /    \/ (_/\
\_)(_/(____) (__) \__/(__\_)(__)\_/\_/\____/`

const (
	CommandCategoryGeneral      = "general"
	CommandCategoryIntegrations = "integrations"
	CommandCategoryResource     = "resource"
)

var helpColors = terminal.Colorizer{}
var browserShellEnabled bool

func ConfigureHelpFeatures(features terminal.Features) {
	helpColors = terminal.NewColorizer(features)
}

func RegisterTemplateFuncs() {
	cobra.AddTemplateFunc("renderCommandSection", renderCommandSection)
	cobra.AddTemplateFunc("renderIntegrationSection", renderIntegrationSection)
	cobra.AddTemplateFunc("hasCommandCategory", hasCommandCategory)
	cobra.AddTemplateFunc("commandAnnotation", CommandAnnotation)
	cobra.AddTemplateFunc("helpHeading", helpHeading)
}

func ConfigureCommand(command *cobra.Command) {
	command.SetHelpTemplate(HelpTemplate())
	command.SetUsageTemplate(UsageTemplate())
}

func SetCommandCategory(command *cobra.Command, category string) {
	SetCommandAnnotation(command, "metorial:command-category", category)
}

func SetCommandAnnotation(command *cobra.Command, key, value string) {
	if command.Annotations == nil {
		command.Annotations = map[string]string{}
	}
	command.Annotations[key] = value
}

func RootLongDescription() string {
	if BrowserShellEnabled() {
		return helpColors.Bold(METORIAL_2) + "\n\n" + strings.TrimSpace(`
Welcome to the Metorial browser shell. Use it to browse providers, manage
deployments, configs, identities, sessions, integrations, MCP tools, and send
raw API requests from the browser.

Start with "metorial providers list" or "metorial deployments list" to explore
core resources, "metorial sessions list" to inspect active sessions,
"metorial integrations list" or "metorial integrations catalog list" to work
with integrations, and "metorial fetch" for raw authenticated API requests.
`)
	}

	return helpColors.Bold(METORIAL_2) + "\n\n" + strings.TrimSpace(`
Welcome to the Metorial CLI! Use it to browse providers, manage deployments,
configs, identities, and sessions, work with integrations and MCP tools, send
raw API requests, and bootstrap example projects.

Start with "metorial login" to sign in, "metorial providers list" or
"metorial deployments list" to explore core resources, "metorial sessions list"
to inspect active sessions, "metorial integrations list" or
"metorial integrations catalog list" to work with integrations, "metorial fetch"
for raw authenticated API requests, "metorial example list" to clone official
examples, and "metorial open" to launch the dashboard in a browser.
`)
}

func BrowserShellEnabled() bool {
	return browserShellEnabled
}

func SetBrowserShellEnabled(enabled bool) {
	browserShellEnabled = enabled
}

func HelpTemplate() string {
	return `{{with (or .Long .Short)}}{{.}}

{{end}}{{helpHeading "Usage:"}}
  {{.UseLine}}{{if .HasAvailableSubCommands}}{{if hasCommandCategory .Commands "general"}}

{{helpHeading "Commands:"}}
{{renderCommandSection .Commands "general"}}{{end}}{{if hasCommandCategory .Commands "integrations"}}

{{helpHeading "Integrations:"}}
{{renderIntegrationSection .Commands}}{{end}}{{if hasCommandCategory .Commands "resource"}}

{{helpHeading "Resource Admin Commands:"}}
{{renderCommandSection .Commands "resource"}}{{end}}{{end}}{{if commandAnnotation . "metorial:arguments"}}

{{helpHeading "Arguments:"}}
{{commandAnnotation . "metorial:arguments"}}{{end}}{{if .HasAvailableLocalFlags}}

{{helpHeading "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{helpHeading "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasExample}}

{{helpHeading "Examples:"}}
{{.Example}}{{end}}{{if commandAnnotation . "metorial:see-also"}}

{{helpHeading "See Also:"}}
{{commandAnnotation . "metorial:see-also"}}{{end}}
`
}

func UsageTemplate() string {
	return `{{helpHeading "Usage:"}}
  {{.UseLine}}{{if .HasAvailableSubCommands}}{{if hasCommandCategory .Commands "general"}}

{{helpHeading "Commands:"}}
{{renderCommandSection .Commands "general"}}{{end}}{{if hasCommandCategory .Commands "integrations"}}

{{helpHeading "Integrations:"}}
{{renderIntegrationSection .Commands}}{{end}}{{if hasCommandCategory .Commands "resource"}}

{{helpHeading "Resource Admin Commands:"}}
{{renderCommandSection .Commands "resource"}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{helpHeading "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{helpHeading "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`
}

func renderCommandSection(commands []*cobra.Command, category string) string {
	var buffer bytes.Buffer
	width := 0

	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) != category {
			continue
		}
		width = MaxInt(width, len(command.Name()))
	}

	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) != category {
			continue
		}

		padding := strings.Repeat(" ", width-len(command.Name()))
		_, _ = fmt.Fprintf(&buffer, "  %s%s  %s\n", helpColors.Accent(command.Name()), padding, command.Short)
	}

	return strings.TrimRight(buffer.String(), "\n")
}

func renderIntegrationSection(commands []*cobra.Command) string {
	var buffer bytes.Buffer
	width := 0

	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) != CommandCategoryIntegrations {
			continue
		}
		width = MaxInt(width, len(command.Name()))
	}

	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) != CommandCategoryIntegrations {
			continue
		}

		padding := strings.Repeat(" ", width-len(command.Name()))
		_, _ = fmt.Fprintf(&buffer, "  %s%s  %s\n", helpColors.Accent(command.Name()), padding, command.Short)

		summary := CommandAnnotation(command, "metorial:help-summary")
		if strings.TrimSpace(summary) == "" {
			continue
		}

		for _, line := range strings.Split(summary, "\n") {
			line = strings.TrimRight(line, " ")
			if strings.TrimSpace(line) == "" {
				continue
			}
			_, _ = fmt.Fprintf(&buffer, "    %s\n", line)
		}
	}

	return strings.TrimRight(buffer.String(), "\n")
}

func hasCommandCategory(commands []*cobra.Command, category string) bool {
	for _, command := range commands {
		if !command.IsAvailableCommand() || command.Hidden {
			continue
		}
		if commandCategory(command) == category {
			return true
		}
	}

	return false
}

func commandCategory(command *cobra.Command) string {
	if command.Annotations != nil {
		if category := strings.TrimSpace(command.Annotations["metorial:command-category"]); category != "" {
			return category
		}
	}

	return CommandCategoryGeneral
}

func CommandAnnotation(command *cobra.Command, key string) string {
	if command == nil || command.Annotations == nil {
		return ""
	}

	return strings.TrimRight(command.Annotations[key], "\n")
}

func helpHeading(value string) string {
	return helpColors.Bold(value)
}
