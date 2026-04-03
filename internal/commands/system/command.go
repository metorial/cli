package system

import (
	"fmt"

	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
	"github.com/metorial/cli/internal/version"
	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(command *cobra.Command, args []string) {
			_, _ = fmt.Fprintf(
				command.OutOrStdout(),
				"metorial %s\ncommit: %s\nbuilt: %s\n",
				version.Version,
				version.Commit,
				version.Date,
			)
		},
	}
}

func NewFeedbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "feedback",
		Short: "Share feedback or report issues",
		Run: func(command *cobra.Command, args []string) {
			link := terminal.Link("github.com/metorial/cli", config.DefaultFeedback)
			_, _ = fmt.Fprintf(command.OutOrStdout(), "Share feedback at %s\n", link)
		},
	}
}

func NewOpenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the Metorial platform in your browser",
		RunE: func(command *cobra.Command, args []string) error {
			platformURL, err := config.ResolvePlatformURL()
			if err != nil {
				return err
			}

			if !browser.Supported() {
				_, _ = fmt.Fprintf(command.OutOrStdout(), "Open this URL in your browser: %s\n", platformURL)
				return nil
			}

			if err := browser.Open(platformURL); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(command.OutOrStdout(), "Opened %s\n", platformURL)
			return nil
		},
	}
}
