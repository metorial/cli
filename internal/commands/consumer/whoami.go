package consumer

import (
	"fmt"
	"io"

	"github.com/metorial/cli/internal/consumerapi"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

type whoamiResult struct {
	Profile    *consumerapi.Profile `json:"profile"`
	PortalID   string               `json:"portal_id,omitempty"`
	PortalName string               `json:"portal_name,omitempty"`
	InstanceID string               `json:"instance_id,omitempty"`
}

func newWhoamiCommand(env commandEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the signed-in consumer, portal, and groups",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := env.ctx.ResolveRuntime()
			if err != nil {
				return err
			}
			client, err := consumerapi.New(runtime)
			if err != nil {
				return err
			}

			profile, err := client.GetProfile()
			if err != nil {
				return err
			}

			result := whoamiResult{Profile: profile, InstanceID: runtime.InstanceID}
			if runtime.Profile != nil {
				result.PortalID = runtime.Profile.PortalID
				result.PortalName = runtime.Profile.PortalName
				result.InstanceID = firstNonEmpty(runtime.Profile.InstanceID, result.InstanceID)
			}

			return env.write(command, result, func(writer io.Writer, features terminal.Features) error {
				return renderWhoami(writer, features, result)
			})
		},
	}
}

func renderWhoami(writer io.Writer, features terminal.Features, result whoamiResult) error {
	colors := terminal.NewColorizer(features)
	profile := result.Profile

	renderTitle(writer, features, firstNonEmpty(profile.Name, profile.Email, profile.ID), profile.Email)
	_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Muted("Consumer:"), profile.ID)
	if portal := firstNonEmpty(result.PortalName, result.PortalID); portal != "" {
		_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Muted("Portal:  "), portal)
	}
	if result.InstanceID != "" {
		_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Muted("Instance:"), result.InstanceID)
	}

	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Accent("Groups"))
	if len(profile.Groups) == 0 {
		_, _ = fmt.Fprintln(writer, "(none)")
	}
	for _, assignment := range profile.Groups {
		line := assignment.Group.Name
		if assignment.Group.IsDefault {
			line += colors.Muted(" (default)")
		}
		if assignment.AssignedVia != "" {
			line += colors.Muted(" via " + assignment.AssignedVia)
		}
		_, _ = fmt.Fprintf(writer, "  %s\n", line)
	}

	return renderTips(writer, features, []string{"metorial catalog list", "metorial list"})
}
