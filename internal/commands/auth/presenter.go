package auth

import (
	"fmt"
	"io"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
)

func renderLoginWaitingScreenWithFeatures(writer io.Writer, features terminal.Features) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Bold("Welcome to the Metorial CLI!"))
	_, _ = fmt.Fprintln(writer)
}

func renderLoginAuthScreen(writer io.Writer, features terminal.Features, authURL, userCode string, browserOpenSupported bool) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Notice("Please sign in to your Metorial account."))
	_, _ = fmt.Fprintln(writer)
	_ = output.RenderBox(writer, []string{
		authURL,
		"Verification Code: " + userCode,
	}, output.BoxOptions{
		MaxWidth: features.Width,
		Unicode:  features.HasUnicode,
	})
	_, _ = fmt.Fprintln(writer)

	if browserOpenSupported {
		_, _ = fmt.Fprintln(writer, colors.Muted("Opening browser for Metorial authentication."))
	}
}

func renderLoginSuccess(writer io.Writer, features terminal.Features) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Success("Logged in to Metorial successfully."))
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Bold("Get started by running `metorial`."))
}

func renderLogoutSuccess(writer io.Writer, features terminal.Features, removedProfile config.Profile, nextProfile *config.Profile) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Success("Logged out from Metorial successfully."))
	_, _ = fmt.Fprintln(writer)
	scopeLabel, subjectLabel := profileLabels(removedProfile.Subject())
	_ = output.RenderBox(writer, []string{
		"Removed Profile: " + removedProfile.Name,
		scopeLabel + ": " + profileScopeValue(removedProfile),
		subjectLabel + ": " + profileSubjectValue(removedProfile),
	}, output.BoxOptions{
		MaxWidth: features.Width,
		Unicode:  features.HasUnicode,
	})
	_, _ = fmt.Fprintln(writer)

	if nextProfile != nil {
		_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Notice("Active profile:"), colors.Bold(nextProfile.Name))
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, colors.Muted("Continue with the Metorial CLI using this profile."))
		return
	}

	_, _ = fmt.Fprintln(writer, colors.Bold("No profiles remain on this machine."))
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Muted("Run `metorial login` to add a new profile."))
}

func profileLabels(subjectType string) (string, string) {
	if config.NormalizeSubjectType(subjectType) == config.SubjectTypeConsumer {
		return "Portal", "Consumer"
	}
	return "Organization", "User"
}

func profileScopeValue(profile config.Profile) string {
	if profile.Subject() == config.SubjectTypeConsumer {
		return commandutil.FirstNonEmpty(profile.PortalName, profile.PortalID, profile.InstanceID)
	}
	return commandutil.FirstNonEmpty(profile.OrgName, profile.OrgID)
}

func profileSubjectValue(profile config.Profile) string {
	if profile.Subject() == config.SubjectTypeConsumer {
		return commandutil.FirstNonEmpty(profile.ConsumerEmail, profile.ConsumerName, profile.ConsumerProfileID)
	}
	return commandutil.FirstNonEmpty(profile.UserEmail, profile.UserName, profile.UserID)
}
