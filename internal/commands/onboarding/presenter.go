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

	if isAgent {
		writeAgentInstructions(writer, rootName, profile)
		return
	}

	_, _ = fmt.Fprintln(writer, colors.Notice("This organization is not claimed yet. Claim it to keep it and invite your team."))
	_, _ = fmt.Fprintln(writer, colors.Muted("Open this link and sign in to keep it and invite your team:"))
	_, _ = fmt.Fprintln(writer, response.Claim.URL)
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

	_, _ = fmt.Fprintln(writer, "You can continue setup now. Include the claim link in your final reply so the user can take ownership:")
	_, _ = fmt.Fprintln(writer, profile.ClaimLink(time.Now()))
	_, _ = fmt.Fprintf(writer, "If the user explicitly requests email delivery, run `%s claim <email>`.\n", rootName)
}
