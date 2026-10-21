package onboarding

import (
	"fmt"
	"io"
	"time"

	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
)

func renderSignupSuccess(writer io.Writer, features terminal.Features, response *auth.SignupResponse, isAgent bool, profile config.Profile, rootName string) {
	colors := terminal.NewColorizer(features)
	boxOptions := output.BoxOptions{MaxWidth: features.Width, Unicode: features.HasUnicode}

	_, _ = fmt.Fprintln(writer, colors.Success("Your Metorial organization is ready."))
	_, _ = fmt.Fprintln(writer)
	details := []string{
		"Organization: " + commandutil.FirstNonEmpty(response.Organization.Slug, response.Organization.ID),
		"Project: " + commandutil.FirstNonEmpty(response.Project.Name, response.Project.ID),
		"Instance: " + commandutil.FirstNonEmpty(response.Instance.Name, response.Instance.ID),
	}
	if response.Region != "" {
		details = append(details, "Region: "+response.Region)
	}
	_ = output.RenderBox(writer, details, boxOptions)
	_, _ = fmt.Fprintln(writer)

	_, _ = fmt.Fprintln(writer, colors.Notice("This organization is not claimed yet. Claim it to keep it and invite your team."))
	_, _ = fmt.Fprintln(writer, colors.Muted("Open this link and sign in to keep it and invite your team:"))
	_, _ = fmt.Fprintln(writer, response.Claim.URL)

	if isAgent {
		_, _ = fmt.Fprintln(writer)
		writeAgentInstructions(writer, rootName, profile)
	}
}

func renderClaimEmailSent(writer io.Writer, features terminal.Features, profile config.Profile, email string) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Success("Claim link sent to"), colors.Bold(email))
	_, _ = fmt.Fprintln(writer, colors.Muted("Metorial will send a daily reminder until the organization is claimed."))
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, profile.ClaimLink(time.Now()))
}

func renderClaimLink(writer io.Writer, features terminal.Features, profile config.Profile) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintf(writer, "%s %s\n", colors.Notice("Claim organization"), colors.Bold(commandutil.FirstNonEmpty(profile.OrgSlug, profile.OrgID)))
	_, _ = fmt.Fprintln(writer, colors.Muted("Open this link and sign in to claim it:"))
	_, _ = fmt.Fprintln(writer, profile.ClaimLink(time.Now()))
}

func renderClaimedLine(writer io.Writer, features terminal.Features, profile config.Profile) {
	colors := terminal.NewColorizer(features)
	user := commandutil.FirstNonEmpty(profile.UserEmail, profile.UserName, profile.UserID)

	_, _ = fmt.Fprintf(
		writer,
		"%s Organization %s was claimed by %s, you're now logged in as %s.\n",
		colors.Success("[metorial]"),
		commandutil.FirstNonEmpty(profile.OrgSlug, profile.OrgID),
		user,
		user,
	)
}

func writeAgentInstructions(writer io.Writer, rootName string, profile config.Profile) {
	orgRef := commandutil.FirstNonEmpty(profile.OrgSlug, profile.OrgID)

	_, _ = fmt.Fprintf(writer, "[metorial] This CLI profile uses an UNCLAIMED Metorial organization (%s).\n", orgRef)

	if profile.ClaimEmail != "" {
		_, _ = fmt.Fprintf(writer, "A claim link was emailed to %s. Ask the user to open it and sign in, or give them this link directly:\n", profile.ClaimEmail)
		_, _ = fmt.Fprintf(writer, "%s\n", profile.ClaimLink(time.Now()))
		_, _ = fmt.Fprintf(writer, "To send it to a different address, run `%s claim <email>`.\n", rootName)
		return
	}

	_, _ = fmt.Fprintln(writer, "Before continuing, ask the user to claim it. Ask them for the email address they want to use with Metorial:")
	if profile.ClaimEmailHint != "" {
		source := commandutil.FirstNonEmpty(profile.ClaimEmailHintSource, "this machine")
		_, _ = fmt.Fprintf(writer, "- A likely address is %s (found in %s). Do not assume it is right: confirm it with the user first.\n", profile.ClaimEmailHint, source)
	}
	_, _ = fmt.Fprintf(writer, "- Once the user gave you their email, run `%s claim <email>` to send them the claim link.\n", rootName)
	_, _ = fmt.Fprintf(writer, "- If they would rather not share it, give them this link to claim the organization themselves: %s\n", profile.ClaimLink(time.Now()))
}
